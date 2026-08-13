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
	"termdevtools/i18n"
)

// ConnectResult gathers what's needed to start the main application after a
// successful connection.
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

// highlightForm wraps tview.Form to make the focused field stand out.
// tview.Form reapplies its own uniform style (background/text) to ALL its
// fields on every frame (in Form.Draw, via SetFormAttributes) — a
// per-field customization set elsewhere (e.g. via SetFocusFunc) is therefore
// systematically overwritten on the next refresh. Here, we redraw only the
// focused field afterwards, with inverted colors, which survives the next
// frame since it's repeated on every Draw().
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

// connectScreen implements the connection screen: list of known clusters
// (§3.0) + new-connection/secrets form. See SPEC.md §3.0, §5.
type connectScreen struct {
	tapp *tview.Application
	cfg  *config.Config
	msgs *i18n.Strings
	on   func(ConnectResult)

	pages   *tview.Pages
	list    *tview.List
	message *tview.TextView
}

// BuildConnectPage builds the connection screen. onConnected is called
// (from the UI thread) once a connection has been successfully established.
func BuildConnectPage(tapp *tview.Application, cfg *config.Config, onConnected func(ConnectResult)) tview.Primitive {
	cs := &connectScreen{
		tapp:    tapp,
		cfg:     cfg,
		msgs:    i18n.For(cfg.Language),
		on:      onConnected,
		pages:   tview.NewPages(),
		list:    tview.NewList().ShowSecondaryText(true),
		message: tview.NewTextView().SetDynamicColors(true),
	}
	cs.list.SetBorder(true).SetTitle(cs.msgs.ConnectListTitle)
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
	cs.list.AddItem(cs.msgs.NewConnectionLabel, cs.msgs.NewConnectionSecondary, 0, func() { cs.openForm(nil) })
	cs.list.AddItem(cs.msgs.QuitLabel, "", 0, func() { cs.tapp.Stop() })
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
	msgs := cs.msgs
	cluster := config.Cluster{
		AuthType: config.AuthNone,
		TLS:      config.TLS{Verify: true, CAFile: cs.cfg.DefaultCADir, ClientCert: cs.cfg.DefaultClientCertDir, ClientKey: cs.cfg.DefaultClientCertDir},
	}
	title := msgs.NewConnectionFormTitle
	if existing != nil {
		cluster = *existing
		title = fmt.Sprintf(msgs.ConnectFormTitleFmt, existing.URL)
	}

	url := cluster.URL
	authType := cluster.AuthType
	username, apiKeyID := cluster.Username, cluster.APIKeyID
	verify := cluster.TLS.Verify
	caFile, clientCert, clientKey := cluster.TLS.CAFile, cluster.TLS.ClientCert, cluster.TLS.ClientKey
	var password, apiKeySecret, keyPassphrase string

	authOptions := []string{config.AuthNone, config.AuthBasic, config.AuthAPIKey, config.AuthMTLS}
	authLabels := []string{msgs.AuthNone, msgs.AuthBasic, msgs.AuthAPIKey, msgs.AuthMTLS}
	authIndex := indexOf(authOptions, authType)
	if authIndex < 0 {
		authIndex = 0
	}

	form := newHighlightForm()
	form.SetBorder(true).SetTitle(title)
	// Without SetCancelFunc, tview.Form falls back to a default behavior
	// (return to the first field) that turned out to freeze focus in
	// practice (e.g. Esc on the authentication dropdown). So we give Esc an
	// explicit, already-tested behavior instead: return to the list.
	form.SetCancelFunc(func() { cs.pages.SwitchToPage("list") })

	// The first 2 fields (URL, Authentication) are static and never
	// rebuilt, so as not to lose focus/cursor while typing. Everything
	// after that (auth-specific + TLS) is dynamically rebuilt by
	// refreshDynamicFields, showing only the fields relevant to the current
	// URL scheme and auth type.
	const staticItemCount = 2
	const authDropdownIndex = 1
	lastIsHTTPS := urlLooksHTTPS(url)
	var refreshDynamicFields func()

	if existing == nil {
		form.AddInputField(msgs.FieldURL, url, 60, nil, func(v string) {
			url = v
			if isHTTPS := urlLooksHTTPS(url); isHTTPS != lastIsHTTPS {
				lastIsHTTPS = isHTTPS
				refreshDynamicFields()
			}
		})
	} else {
		form.AddTextView(msgs.FieldURLReadOnly, url, 60, 1, true, false)
	}

	form.AddDropDown(msgs.AuthFieldLabel, authLabels, authIndex, func(_ string, index int) {
		authType = authOptions[index]
		// AddDropDown calls this callback once synchronously, at
		// construction time, to set the initial value —
		// refreshDynamicFields isn't assigned yet at that point.
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
			form.AddInputField(msgs.FieldUsername, username, 40, nil, func(v string) { username = v })
			form.AddPasswordField(msgs.FieldPassword, "", 40, '*', func(v string) { password = v })
		case config.AuthAPIKey:
			form.AddInputField(msgs.FieldAPIKeyID, apiKeyID, 40, nil, func(v string) { apiKeyID = v })
			form.AddPasswordField(msgs.FieldAPIKeySecret, "", 40, '*', func(v string) { apiKeySecret = v })
		case config.AuthMTLS:
			// A client certificate is part of the TLS handshake: doesn't
			// make sense if the connection isn't over https.
			if isHTTPS {
				form.AddInputField(msgs.FieldClientCert, clientCert, 60, nil, func(v string) { clientCert = v })
				form.AddInputField(msgs.FieldClientKey, clientKey, 60, nil, func(v string) { clientKey = v })
				form.AddPasswordField(msgs.FieldKeyPassphrase, "", 40, '*', func(v string) { keyPassphrase = v })
			}
		}

		if isHTTPS {
			form.AddInputField(msgs.FieldCAFile, caFile, 60, nil, func(v string) { caFile = v })
			form.AddCheckbox(msgs.FieldVerifyTLS, verify, func(v bool) { verify = v })
		}
	}
	refreshDynamicFields()

	form.AddButton(msgs.ButtonConnect, func() {
		cl := config.Cluster{
			URL: url, AuthType: authType,
			Username: username, APIKeyID: apiKeyID,
			TLS: config.TLS{Verify: verify, CAFile: caFile, ClientCert: clientCert, ClientKey: clientKey},
		}
		cs.attemptConnect(cl, connectSecrets{password: password, apiKeySecret: apiKeySecret, keyPassphrase: keyPassphrase})
	})
	form.AddButton(msgs.ButtonCancel, func() {
		cs.pages.SwitchToPage("list")
	})

	return form
}

func (cs *connectScreen) attemptConnect(cl config.Cluster, secrets connectSecrets) {
	msgs := cs.msgs
	if cl.URL == "" {
		cs.setMessage(msgs.ErrURLRequired, "red")
		return
	}
	cs.setMessage(msgs.StatusConnecting, "yellow")

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
				cs.setMessage(fmt.Sprintf(msgs.ErrConnectFailedFmt, err), "red")
				return
			}
			if result.StatusCode >= 400 {
				cs.setMessage(fmt.Sprintf(msgs.ErrClusterHTTPFmt, result.StatusCode), "red")
				return
			}

			cs.cfg.Promote(cl)
			if err := cs.cfg.Save(); err != nil {
				cs.setMessage(fmt.Sprintf(msgs.WarnConnectedSaveFailedFmt, err), "yellow")
			}

			cs.on(ConnectResult{Client: client, Cluster: cl, DisplayUser: displayUserFor(cl, msgs)})
		})
	}()
}

func displayUserFor(cl config.Cluster, msgs *i18n.Strings) string {
	switch cl.AuthType {
	case config.AuthBasic:
		return cl.Username
	case config.AuthAPIKey:
		return "api_key:" + cl.APIKeyID
	case config.AuthMTLS:
		return "mTLS"
	default:
		return msgs.DisplayUserNoAuth
	}
}

func (cs *connectScreen) setMessage(msg, color string) {
	cs.message.SetText(fmt.Sprintf("[%s]%s[white]", color, tview.Escape(msg)))
}

// setFieldColors changes a FormItem's field colors — tview has no common
// method for this (each type returns its own concrete type), hence the
// switch. Used by highlightForm.Draw to make the focused field stand out.
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

// urlLooksHTTPS determines whether TLS-related fields should be offered:
// only a URL explicitly in http:// hides them, everything else (https://,
// a still-incomplete/empty URL...) shows them by default.
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
