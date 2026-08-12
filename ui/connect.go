package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/esclient"
)

const newConnectionLabel = "+ Nouvelle connexion"

// ConnectResult regroupe ce qu'il faut pour démarrer l'application principale
// après une connexion réussie.
type ConnectResult struct {
	Client      *esclient.Client
	Cluster     config.Cluster
	DisplayUser string
}

type connectSecrets struct {
	password      string
	apiKeySecret  string
	keyPassphrase string
}

// highlightForm enrobe tview.Form pour faire ressortir le champ ayant le
// focus. tview.Form réapplique son propre style uniforme (fond/texte) à
// TOUS ses champs à chaque frame (dans Form.Draw, via SetFormAttributes) —
// une personnalisation par champ posée ailleurs (ex. via SetFocusFunc) est
// donc systématiquement écrasée dès le rafraîchissement suivant. On
// redessine ici, après coup, uniquement le champ focalisé avec des couleurs
// inversées, ce qui survit au prochain frame car répété à chaque Draw().
type highlightForm struct {
	*tview.Form
}

func newHighlightForm() *highlightForm {
	return &highlightForm{Form: tview.NewForm()}
}

func (h *highlightForm) Draw(screen tcell.Screen) {
	h.Form.Draw(screen)

	for i := 0; i < h.Form.GetFormItemCount(); i++ {
		item := h.Form.GetFormItem(i)
		if !item.HasFocus() {
			continue
		}
		setFieldColors(item, tview.Styles.PrimaryTextColor, tview.Styles.ContrastBackgroundColor)
		item.Draw(screen)
		break
	}
}

// connectScreen implémente l'écran de connexion : liste des clusters connus
// (§3.0) + formulaire de nouvelle connexion / secrets. Voir SPEC.md §3.0, §5.
type connectScreen struct {
	tapp *tview.Application
	cfg  *config.Config
	on   func(ConnectResult)

	pages   *tview.Pages
	list    *tview.List
	message *tview.TextView
}

// BuildConnectPage construit l'écran de connexion. onConnected est appelé
// (depuis le thread UI) une fois une connexion établie avec succès.
func BuildConnectPage(tapp *tview.Application, cfg *config.Config, onConnected func(ConnectResult)) tview.Primitive {
	cs := &connectScreen{
		tapp:    tapp,
		cfg:     cfg,
		on:      onConnected,
		pages:   tview.NewPages(),
		list:    tview.NewList().ShowSecondaryText(true),
		message: tview.NewTextView().SetDynamicColors(true),
	}
	cs.list.SetBorder(true).SetTitle(" TermDevTools — connexion ")
	cs.refreshList()
	cs.pages.AddPage("list", cs.listLayout(), true, true)
	return cs.pages
}

func (cs *connectScreen) listLayout() tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cs.list, 0, 1, true).
		AddItem(cs.message, 1, 0, false)
}

func (cs *connectScreen) refreshList() {
	cs.list.Clear()
	for _, cl := range cs.cfg.Clusters {
		cl := cl
		secondary := fmt.Sprintf("auth: %s", cl.AuthType)
		cs.list.AddItem(cl.URL, secondary, 0, func() { cs.openForm(&cl) })
	}
	cs.list.AddItem(newConnectionLabel, "saisir une nouvelle connexion", 0, func() { cs.openForm(nil) })
	cs.list.AddItem("Quitter", "", 0, func() { cs.tapp.Stop() })
}

func (cs *connectScreen) openForm(existing *config.Cluster) {
	form := cs.buildForm(existing)
	cs.setMessage("", "white")
	cs.pages.AddAndSwitchToPage("form", cs.formLayout(form), true)
}

func (cs *connectScreen) formLayout(form tview.Primitive) tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(cs.message, 1, 0, false)
}

func (cs *connectScreen) buildForm(existing *config.Cluster) *highlightForm {
	cluster := config.Cluster{
		AuthType: config.AuthNone,
		TLS:      config.TLS{Verify: true, CAFile: cs.cfg.DefaultCADir, ClientCert: cs.cfg.DefaultClientCertDir, ClientKey: cs.cfg.DefaultClientCertDir},
	}
	title := " Nouvelle connexion "
	if existing != nil {
		cluster = *existing
		title = fmt.Sprintf(" Connexion à %s ", existing.URL)
	}

	url := cluster.URL
	authType := cluster.AuthType
	username, apiKeyID := cluster.Username, cluster.APIKeyID
	verify := cluster.TLS.Verify
	caFile, clientCert, clientKey := cluster.TLS.CAFile, cluster.TLS.ClientCert, cluster.TLS.ClientKey
	var password, apiKeySecret, keyPassphrase string

	authOptions := []string{config.AuthNone, config.AuthBasic, config.AuthAPIKey, config.AuthMTLS}
	authLabels := []string{"aucune", "Basic Auth", "API Key", "certificat client (mTLS)"}
	authIndex := indexOf(authOptions, authType)
	if authIndex < 0 {
		authIndex = 0
	}

	form := newHighlightForm()
	form.SetBorder(true).SetTitle(title)
	// Sans SetCancelFunc, tview.Form retombe sur un comportement par défaut
	// (retour au premier champ) qui s'est avéré geler le focus en pratique
	// (ex. Echap sur la liste déroulante d'authentification). On donne donc
	// à Echap un comportement explicite et déjà testé : retour à la liste.
	form.SetCancelFunc(func() { cs.pages.SwitchToPage("list") })

	// Les 2 premiers champs (URL, Authentification) sont statiques et ne
	// sont jamais reconstruits, pour ne pas perdre le focus/curseur pendant
	// la saisie. Tout ce qui suit (auth spécifique + TLS) est reconstruit
	// dynamiquement par refreshDynamicFields, en ne montrant que les champs
	// pertinents pour le schéma d'URL et le type d'auth courants.
	const staticItemCount = 2
	const authDropdownIndex = 1
	lastIsHTTPS := urlLooksHTTPS(url)
	var refreshDynamicFields func()

	if existing == nil {
		form.AddInputField("URL (https://host:port)", url, 60, nil, func(v string) {
			url = v
			if isHTTPS := urlLooksHTTPS(url); isHTTPS != lastIsHTTPS {
				lastIsHTTPS = isHTTPS
				refreshDynamicFields()
			}
		})
	} else {
		form.AddTextView("URL", url, 60, 1, true, false)
	}

	form.AddDropDown("Authentification", authLabels, authIndex, func(_ string, index int) {
		authType = authOptions[index]
		// AddDropDown appelle ce callback une première fois de façon
		// synchrone, à la construction, pour fixer la valeur initiale —
		// refreshDynamicFields n'est pas encore assignée à ce moment-là.
		if refreshDynamicFields == nil {
			return
		}
		refreshDynamicFields()
		form.SetFocus(authDropdownIndex)
	})

	refreshDynamicFields = func() {
		for form.GetFormItemCount() > staticItemCount {
			form.RemoveFormItem(staticItemCount)
		}

		isHTTPS := urlLooksHTTPS(url)

		switch authType {
		case config.AuthBasic:
			form.AddInputField("Username", username, 40, nil, func(v string) { username = v })
			form.AddPasswordField("Mot de passe", "", 40, '*', func(v string) { password = v })
		case config.AuthAPIKey:
			form.AddInputField("API Key ID", apiKeyID, 40, nil, func(v string) { apiKeyID = v })
			form.AddPasswordField("API Key secret", "", 40, '*', func(v string) { apiKeySecret = v })
		case config.AuthMTLS:
			// Un certificat client fait partie de la poignée de main TLS :
			// pas de sens si la connexion n'est pas en https.
			if isHTTPS {
				form.AddInputField("Certificat client", clientCert, 60, nil, func(v string) { clientCert = v })
				form.AddInputField("Clé privée client", clientKey, 60, nil, func(v string) { clientKey = v })
				form.AddPasswordField("Passphrase de la clé (si chiffrée)", "", 40, '*', func(v string) { keyPassphrase = v })
			}
		}

		if isHTTPS {
			form.AddInputField("Fichier CA", caFile, 60, nil, func(v string) { caFile = v })
			form.AddCheckbox("Vérifier le certificat serveur (TLS)", verify, func(v bool) { verify = v })
		}
	}
	refreshDynamicFields()

	form.AddButton("Se connecter", func() {
		cl := config.Cluster{
			URL: url, AuthType: authType,
			Username: username, APIKeyID: apiKeyID,
			TLS: config.TLS{Verify: verify, CAFile: caFile, ClientCert: clientCert, ClientKey: clientKey},
		}
		cs.attemptConnect(cl, connectSecrets{password: password, apiKeySecret: apiKeySecret, keyPassphrase: keyPassphrase})
	})
	form.AddButton("Annuler", func() {
		cs.pages.SwitchToPage("list")
	})

	return form
}

func (cs *connectScreen) attemptConnect(cl config.Cluster, secrets connectSecrets) {
	if cl.URL == "" {
		cs.setMessage("L'URL est obligatoire.", "red")
		return
	}
	cs.setMessage("Connexion en cours...", "yellow")

	timeout := time.Duration(cs.cfg.DefaultTimeoutSeconds) * time.Second
	params := esclient.Params{
		URL: cl.URL, AuthType: cl.AuthType,
		Username: cl.Username, Password: secrets.password,
		APIKeyID: cl.APIKeyID, APIKeySecret: secrets.apiKeySecret,
		Verify: cl.TLS.Verify, CAFile: cl.TLS.CAFile,
		ClientCert: cl.TLS.ClientCert, ClientKey: cl.TLS.ClientKey, KeyPassphrase: secrets.keyPassphrase,
		Timeout: timeout,
	}

	go func() {
		client, err := esclient.New(params)
		var result *esclient.Result
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			result, err = client.Execute(ctx, "GET", "/", nil)
		}

		cs.tapp.QueueUpdateDraw(func() {
			if err != nil {
				cs.setMessage(fmt.Sprintf("Échec de connexion : %s", err), "red")
				return
			}
			if result.StatusCode >= 400 {
				cs.setMessage(fmt.Sprintf("Le cluster a répondu HTTP %d", result.StatusCode), "red")
				return
			}

			cs.cfg.Promote(cl)
			if err := cs.cfg.Save(); err != nil {
				cs.setMessage(fmt.Sprintf("Connecté, mais échec de sauvegarde de config.yaml : %s", err), "yellow")
			}

			cs.on(ConnectResult{Client: client, Cluster: cl, DisplayUser: displayUserFor(cl)})
		})
	}()
}

func displayUserFor(cl config.Cluster) string {
	switch cl.AuthType {
	case config.AuthBasic:
		return cl.Username
	case config.AuthAPIKey:
		return "api_key:" + cl.APIKeyID
	case config.AuthMTLS:
		return "mTLS"
	default:
		return "(aucune auth)"
	}
}

func (cs *connectScreen) setMessage(msg, color string) {
	cs.message.SetText(fmt.Sprintf("[%s]%s[white]", color, tview.Escape(msg)))
}

// setFieldColors change les couleurs de champ d'un FormItem — pas de
// méthode commune pour ça dans tview (chaque type renvoie son propre type
// concret), d'où le switch. Utilisé par highlightForm.Draw pour faire
// ressortir le champ ayant le focus.
func setFieldColors(item tview.FormItem, bg, text tcell.Color) {
	switch w := item.(type) {
	case *tview.InputField:
		w.SetFieldBackgroundColor(bg).SetFieldTextColor(text)
	case *tview.DropDown:
		w.SetFieldBackgroundColor(bg).SetFieldTextColor(text)
	case *tview.Checkbox:
		w.SetFieldBackgroundColor(bg).SetFieldTextColor(text)
	}
}

// urlLooksHTTPS détermine si les champs liés au TLS doivent être proposés :
// seule une URL explicitement en http:// les masque, tout le reste
// (https://, url encore incomplète/vide...) les affiche par défaut.
func urlLooksHTTPS(url string) bool {
	return !strings.HasPrefix(strings.ToLower(strings.TrimSpace(url)), "http://")
}

func indexOf(options []string, v string) int {
	for i, o := range options {
		if o == v {
			return i
		}
	}
	return -1
}
