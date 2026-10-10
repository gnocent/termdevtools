package ui

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"termdevtools/config"
	"termdevtools/esclient"
	"termdevtools/i18n"
	"termdevtools/refdata"
)

// ConnectResult gathers what's needed to start the main application after a
// successful connection.
type ConnectResult struct {
	Client      *esclient.Client
	Cluster     config.Cluster
	DisplayUser string
	// Target is the cluster's distribution and version, detected from the
	// "GET /" that validated the connection (SPEC.md §3.0) — what reference
	// data (endpoints, _cat columns, recipes) gets selected for.
	Target refdata.Target
	// Warning, when set, is shown in the status bar once the main screen is
	// up: something worth knowing that didn't prevent the connection — an
	// override that had to be ignored, a proxy every request goes through.
	Warning string
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

	// attempt numbers the connection attempts: an answer is only acted on if
	// it belongs to the latest one, and none once connected. Without it a
	// slow cluster given up on (Esc, then another one chosen) would, on
	// finally answering, replace the session opened since — and a second
	// press on Connect would open two.
	attempt   int
	connected bool
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
		message: tview.NewTextView().SetDynamicColors(true).SetWordWrap(true),
	}
	cs.list.SetBorder(true).SetTitle(cs.msgs.ConnectListTitle)
	cs.refreshList()
	cs.pages.AddPage("list", cs.listLayout(), true, true)
	return cs.pages
}

// connectMessageLines is the height of the message under the list and the
// form. More than one line: a connection failure through a proxy says which
// proxy and how to do without it, after an error that is long by itself.
const connectMessageLines = 3

func (cs *connectScreen) listLayout() tview.Primitive {
	return tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cs.list, 0, 1, true).
		AddItem(cs.message, connectMessageLines, 0, false)
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
		AddItem(cs.message, connectMessageLines, 0, false)
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
	// Esc and the Cancel button both give up on an attempt still in progress,
	// if any, and return to the list.
	cancel := func() {
		cs.attempt++
		cs.pages.SwitchToPage("list")
	}
	form.SetCancelFunc(cancel)

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
				form.AddInputField(msgs.FieldClientCert+msgs.BrowseHint, clientCert, 60, nil, func(v string) { clientCert = v })
				cs.attachCertPicker(form, cs.cfg.DefaultClientCertDir, "default_client_cert_dir")
				form.AddInputField(msgs.FieldClientKey+msgs.BrowseHint, clientKey, 60, nil, func(v string) { clientKey = v })
				cs.attachCertPicker(form, cs.cfg.DefaultClientCertDir, "default_client_cert_dir")
				form.AddPasswordField(msgs.FieldKeyPassphrase, "", 40, '*', func(v string) { keyPassphrase = v })
			}
		}

		if isHTTPS {
			form.AddInputField(msgs.FieldCAFile+msgs.BrowseHint, caFile, 60, nil, func(v string) { caFile = v })
			cs.attachCertPicker(form, cs.cfg.DefaultCADir, "default_ca_dir")
			form.AddCheckbox(msgs.FieldVerifyTLS, verify, func(v bool) { verify = v })
		}
	}
	refreshDynamicFields()

	form.AddButton(msgs.ButtonConnect, func() {
		cl := config.Cluster{
			URL: url, AuthType: authType,
			Username: username, APIKeyID: apiKeyID,
			TLS: config.TLS{Verify: verify, CAFile: caFile, ClientCert: clientCert, ClientKey: clientKey},
			// Not form fields: carried over from config.yaml as-is, or
			// Promote would erase them on every reconnection.
			Distribution: cluster.Distribution, Version: cluster.Version,
			Proxy: cluster.Proxy,
		}
		cs.attemptConnect(cl, connectSecrets{password: password, apiKeySecret: apiKeySecret, keyPassphrase: keyPassphrase})
	})
	form.AddButton(msgs.ButtonCancel, cancel)

	return form
}

// certPickerPageName is the tview.Pages key for the certificate-picker
// popup (openCertPicker) — added/removed on top of the "form" page, same
// idiom as the main app's completion list and help overlay.
const certPickerPageName = "certpicker"

// attachCertPicker wires Enter, on the InputField most recently added to
// form, to open a popup listing the files in dir (the configured
// default_ca_dir/default_client_cert_dir, identified by settingName purely
// for the error message when dir is empty) instead of the form's usual
// "confirm and move to the next field" behavior — lets a CA/client-cert/
// client-key path be picked instead of typed from memory (SPEC.md §5).
//
// Intercepted via SetInputCapture rather than SetDoneFunc: InputField's
// internal finish() calls both the "done" and Form's own "finished"
// callback on Enter, and the latter unconditionally advances focus to the
// next field — racing our own SetFocus into the popup right after. Capturing
// the event before InputField.InputHandler ever runs avoids that race
// entirely; every other key (typing, Tab, Backtab, Escape) passes through
// untouched.
func (cs *connectScreen) attachCertPicker(form *highlightForm, dir, settingName string) {
	item := form.GetFormItem(form.GetFormItemCount() - 1)
	field, ok := item.(*tview.InputField)
	if !ok {
		return
	}
	field.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			cs.openCertPicker(dir, settingName, field)
			return nil
		}
		return event
	})
}

// openCertPicker shows a popup — a small file browser — listing the entries
// directly inside dir and, on picking a file, fills field with its full
// path (see attachCertPicker). Enter on a subdirectory browses into it,
// Backspace goes back up to the parent directory, Escape cancels entirely.
// dir isn't a gate: if it doesn't exist (a stale path, or default_ca_dir's
// own Linux-only default not applying here — see config.defaultCertDir),
// the popup falls back to browsing the user's home directory instead of
// dead-ending, so a certificate kept anywhere else can still be reached by
// navigating there, or the field can simply be typed by hand. Reports an
// error in the status message instead of opening an empty popup only when
// dir isn't configured at all (settingName names the config.yaml key to
// set, e.g. "default_ca_dir"), or when even the home-directory fallback
// isn't usable.
func (cs *connectScreen) openCertPicker(dir, settingName string, field *tview.InputField) {
	msgs := cs.msgs
	if strings.TrimSpace(dir) == "" {
		cs.setMessage(fmt.Sprintf(msgs.ErrNoCertDirConfiguredFmt, settingName), "red")
		return
	}

	entries, err := certEntriesIn(dir)
	if os.IsNotExist(err) {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			if homeEntries, homeErr := certEntriesIn(home); homeErr == nil {
				dir, entries, err = home, homeEntries, nil
			}
		}
	}
	if os.IsNotExist(err) {
		// The configured directory doesn't exist, and the home-directory
		// fallback above didn't pan out either (also missing/unreadable) —
		// only then is this worth surfacing, naming the setting to fix
		// rather than a raw filesystem error.
		cs.setMessage(fmt.Sprintf(msgs.ErrCertDirNotFoundFmt, dir), "red")
		return
	}
	if err != nil {
		cs.setMessage(fmt.Sprintf(msgs.ErrLoadFailedFmt, dir, err), "red")
		return
	}
	if len(entries) == 0 {
		cs.setMessage(fmt.Sprintf(msgs.ErrNoCertFilesInDirFmt, dir), "red")
		return
	}

	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true)

	// currentDir tracks whichever directory render last drew — updated only
	// on success, so a read error while navigating (into a subdirectory, or
	// back up via Backspace) reports the error without leaving the popup on
	// a directory it never actually managed to list.
	currentDir := dir
	var render func(d string, entries []certEntry)
	render = func(d string, entries []certEntry) {
		currentDir = d
		list.Clear()
		list.SetTitle(fmt.Sprintf(msgs.CertPickerTitleFmt, currentDir))
		for _, e := range entries {
			e := e
			label := e.name
			if e.isDir {
				label += string(filepath.Separator)
			}
			list.AddItem(label, "", 0, func() {
				target := filepath.Join(currentDir, e.name)
				if !e.isDir {
					field.SetText(target)
					cs.closeCertPicker(field)
					return
				}
				subEntries, err := certEntriesIn(target)
				if err != nil {
					cs.setMessage(fmt.Sprintf(msgs.ErrLoadFailedFmt, target, err), "red")
					return
				}
				render(target, subEntries)
			})
		}
	}
	render(dir, entries)

	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			cs.closeCertPicker(field)
			return nil
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			parent := filepath.Dir(currentDir)
			if parent == currentDir {
				return nil
			}
			parentEntries, err := certEntriesIn(parent)
			if err != nil {
				cs.setMessage(fmt.Sprintf(msgs.ErrLoadFailedFmt, parent, err), "red")
				return nil
			}
			render(parent, parentEntries)
			return nil
		}
		return event
	})

	// Centered popup, margins around it to let the form show through in the
	// background — same nested-Flex idiom as the main app's help overlay
	// (ui/app.go). 70 columns wide: enough to fit a full absolute Windows
	// path (see TestOpenCertPickerFillsFieldWithFullPath) without the popup
	// itself dominating the screen.
	overlay := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 0, 1, false).
			AddItem(list, 70, 0, true).
			AddItem(nil, 0, 1, false),
			0, 9, true).
		AddItem(nil, 0, 1, false)

	cs.pages.AddPage(certPickerPageName, overlay, true, true)
	cs.tapp.SetFocus(list)
}

// closeCertPicker removes the certificate-picker popup and returns focus to
// the field it was opened from — whether closed by picking a file or by
// Escape.
func (cs *connectScreen) closeCertPicker(field *tview.InputField) {
	cs.pages.RemovePage(certPickerPageName)
	cs.tapp.SetFocus(field)
}

// certEntry describes one file or subdirectory as listed by openCertPicker.
type certEntry struct {
	name  string
	isDir bool
}

// certEntriesIn lists the regular files and subdirectories directly inside
// dir (no dotfiles), subdirectories first then files, each group sorted
// alphabetically — what openCertPicker's file browser offers to navigate
// into or pick from.
func certEntriesIn(dir string) ([]certEntry, error) {
	raw, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var dirNames, fileNames []string
	for _, e := range raw {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirNames = append(dirNames, e.Name())
		} else {
			fileNames = append(fileNames, e.Name())
		}
	}
	sort.Strings(dirNames)
	sort.Strings(fileNames)

	entries := make([]certEntry, 0, len(dirNames)+len(fileNames))
	for _, name := range dirNames {
		entries = append(entries, certEntry{name: name, isDir: true})
	}
	for _, name := range fileNames {
		entries = append(entries, certEntry{name: name, isDir: false})
	}
	return entries, nil
}

func (cs *connectScreen) attemptConnect(cl config.Cluster, secrets connectSecrets) {
	msgs := cs.msgs
	if cl.URL == "" {
		cs.setMessage(msgs.ErrURLRequired, "red")
		return
	}
	// "https://user:password@host" would work — and put the password in
	// config.yaml, in the status bar and in every curl command copied: the
	// URL is the one thing about a cluster that is saved and shown as is.
	if parsed, err := url.Parse(cl.URL); err == nil && parsed.User != nil {
		cs.setMessage(msgs.ErrURLCredentials, "red")
		return
	}
	// The proxy this goes through, if any, is named before anything is sent
	// through it — and one that can't be used stops the attempt here.
	proxy, err := esclient.ProxyFor(cl.URL, cl.Proxy)
	if err != nil {
		cs.setMessage(fmt.Sprintf(msgs.ErrConnectFailedFmt, err), "red")
		return
	}
	connecting := msgs.StatusConnecting
	if proxy.URL != "" {
		connecting = fmt.Sprintf(msgs.StatusConnectingViaProxyFmt, proxy.URL, proxyOrigin(proxy))
	}
	cs.setMessage(connecting, "yellow")
	cs.attempt++
	attempt := cs.attempt

	timeout := time.Duration(cs.cfg.DefaultTimeoutSeconds) * time.Second
	params := esclient.Params{
		URL: cl.URL, AuthType: cl.AuthType,
		Username: cl.Username, Password: secrets.password,
		APIKeyID: cl.APIKeyID, APIKeySecret: secrets.apiKeySecret,
		Verify: cl.TLS.Verify, CAFile: cl.TLS.CAFile,
		ClientCert: cl.TLS.ClientCert, ClientKey: cl.TLS.ClientKey, KeyPassphrase: secrets.keyPassphrase,
		Proxy:   cl.Proxy,
		Timeout: timeout,
	}

	go func() {
		client, err := esclient.New(params)
		var result *esclient.Result
		// Only what was actually sent can have failed because of the proxy.
		var sentThrough esclient.ProxyInfo
		if err == nil {
			sentThrough = proxy
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			result, err = client.Execute(ctx, "GET", "/", nil)
		}

		cs.tapp.QueueUpdateDraw(func() {
			if attempt != cs.attempt || cs.connected {
				return // superseded, or given up on: see connectScreen.attempt
			}
			if err != nil {
				cs.setMessage(withProxyHint(fmt.Sprintf(msgs.ErrConnectFailedFmt, err), sentThrough, msgs), "red")
				return
			}
			// A redirection included: it isn't followed (see esclient.New),
			// and what answers isn't the cluster's root.
			if result.StatusCode >= 300 {
				message := fmt.Sprintf(msgs.ErrClusterHTTPFmt, result.StatusCode)
				if location := result.Headers.Get("Location"); location != "" {
					message += " → " + location
				}
				// Through a proxy, the answer may well be the proxy's own.
				cs.setMessage(withProxyHint(message, sentThrough, msgs), "red")
				return
			}
			cs.connected = true

			cs.cfg.Promote(cl)
			if err := cs.cfg.Save(); err != nil {
				cs.setMessage(fmt.Sprintf(msgs.WarnConnectedSaveFailedFmt, err), "yellow")
			}

			target, warning := resolveTarget(result.Body, cl, msgs)
			if proxy.URL != "" {
				// Still worth knowing once connected: every request goes there.
				notice := fmt.Sprintf(msgs.InfoProxyFmt, proxy.URL, proxyOrigin(proxy))
				if warning != "" {
					notice = warning + " · " + notice
				}
				warning = notice
			}
			cs.on(ConnectResult{
				Client: client, Cluster: cl, DisplayUser: displayUserFor(cl, msgs),
				Target: target, Warning: warning,
			})
		})
	}()
}

// resolveTarget detects the cluster's distribution and version from the body
// of its "GET /" response, then applies cl's optional override from
// config.yaml. An invalid override is reported (warning) and ignored rather
// than failing the connection: a typo in config.yaml shouldn't lock anyone
// out of a cluster.
func resolveTarget(rootBody []byte, cl config.Cluster, msgs *i18n.Strings) (target refdata.Target, warning string) {
	detected := refdata.DetectTarget(rootBody)
	target, err := detected.WithOverride(cl.Distribution, cl.Version)
	if err != nil {
		return detected, fmt.Sprintf(msgs.WarnTargetOverrideFmt, err)
	}
	return target, ""
}

// proxyOrigin says where a proxy in use comes from: the environment
// variable that designates it, or the cluster's own setting.
func proxyOrigin(proxy esclient.ProxyInfo) string {
	if proxy.EnvVar != "" {
		return proxy.EnvVar
	}
	return "config.yaml"
}

// withProxyHint puts proxyHint in front of the message of a failed attempt.
// In front, not after: the error itself is often longer than the few lines
// there are to show it in, and what it is cut short of must not be the one
// part that says what to do.
func withProxyHint(message string, proxy esclient.ProxyInfo, msgs *i18n.Strings) string {
	if hint := proxyHint(proxy, msgs); hint != "" {
		return hint + " " + message
	}
	return message
}

// proxyHint is what a failed attempt that went through proxy must say
// besides its error (nothing if it went through none): which proxy, and for
// one the environment designates, how to do without it. A cluster reached
// directly until then — the environment was ignored before 0.7 — otherwise
// fails with nothing in the error itself to tell why.
func proxyHint(proxy esclient.ProxyInfo, msgs *i18n.Strings) string {
	switch {
	case proxy.URL == "":
		return ""
	case proxy.EnvVar != "":
		return fmt.Sprintf(msgs.HintProxyEnvFmt, proxy.URL, proxy.EnvVar)
	default:
		return fmt.Sprintf(msgs.HintProxyConfigFmt, proxy.URL)
	}
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
