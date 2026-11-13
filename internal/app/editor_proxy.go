package app

// The connection editor's Proxy page: the type the host connects through
// (none, SOCKS5, HTTP or a jump host), and for a proxy, the saved proxies
// of that type, one of which it goes through. The proxies are the store's
// (config.Store.Proxies), and so are their passwords, which the dialog
// that adds and changes one asks for: adding, changing and removing a
// proxy here changes it for every host that uses it, as FinalShell's proxy
// list does. Connecting through them is in proxy.go and
// internal/sshx/proxy.go.

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"finallyshell/internal/config"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

// The Proxy page's fields that can be wrong.
const (
	fieldProxyEntry editorField = 100 + iota
	fieldJumpHost
)

// proxyFieldOrder are the Proxy page's fields, in the order they show.
var proxyFieldOrder = []editorField{fieldProxyEntry, fieldJumpHost}

// proxyFields are the Proxy page's state as typed, until OK saves it into
// the host and the store: the saved proxies, the one of the kind chosen
// that the host connects through, and the jump host it goes through first,
// if any.
type proxyFields struct {
	// proxies are the saved proxies, copies of the store's: what the page
	// adds, changes and removes is saved on OK, and dropped on Cancel.
	proxies []*config.ProxyEntry
	table   ui.ListState
	// kind is what the host connects through: none, SOCKS5, HTTP, whose
	// list the page shows, or a jump host.
	kind config.ProxyKind
	// entry is the ID of the saved proxy of kind the host goes through,
	// "" for none chosen yet.
	entry string
	// jump is the ID of the jump host, "" for none: going through another
	// saved host is the other way to reach this one, so it clears entry.
	jump string
	// removed are the IDs of the saved proxies the page removed, which no
	// host uses once it is saved.
	removed []string
	// passwords are the saved proxies' passwords as the page has them, by
	// entry ID, and changed the ones the dialog wrote: OK keeps those in
	// the credential store, as a proxy's password is the proxy's.
	passwords map[string]string
	changed   map[string]bool
	// asked are the entries whose saved password the page asked the
	// credential store for, so that it asks once.
	asked map[string]bool

	dialog  bool
	editing int // the row of proxies the dialog changes, -1 for a new one
	form    proxyForm

	// saved is the host's proxy as the editor opened, whose JSON keys
	// this build does not know saving keeps; nil for none.
	saved *config.ProxyConfig
	// hosts are the saved hosts, which the jump host is one of.
	hosts *config.Store
}

// proxyFieldsOf are the Proxy page's fields for h, nil for a new host.
func (a *App) proxyFieldsOf(h *config.Host) proxyFields {
	pf := proxyFields{hosts: a.hosts, editing: -1, proxies: config.CloneProxies(a.hosts.Proxies),
		passwords: map[string]string{}, changed: map[string]bool{}, asked: map[string]bool{}}
	if h == nil {
		return pf
	}
	if h.Proxy != nil {
		p := h.Proxy
		pf.saved = p
		switch {
		case p.Kind == config.ProxyJump:
			pf.kind, pf.jump = config.ProxyJump, p.JumpHost
		case config.IsProxyServer(p.Kind):
			pf.kind, pf.entry = p.Kind, p.ID
			if p.ID != "" && pf.proxy(p.ID) == nil {
				// A proxy the store does not have, as a hosts.json from
				// another device, which does not carry the list, or a
				// hand-edited file: the page shows it, rather than lose it
				// on OK.
				pf.proxies = append(pf.proxies, p.Entry())
			}
		}
	}
	for _, e := range pf.proxies {
		if pw, ok := a.proxySecrets()[proxySecretKey(e.ID)]; ok {
			pf.passwords[e.ID] = pw
		}
	}
	return pf
}

// index is where the saved proxy with the ID is in the page's list, -1
// for one that is not there.
func (pf *proxyFields) index(id string) int {
	return slices.IndexFunc(pf.proxies, func(e *config.ProxyEntry) bool { return e.ID == id })
}

// proxy is the saved proxy with the ID, nil for "" or one that is not in
// the page's list.
func (pf *proxyFields) proxy(id string) *config.ProxyEntry {
	if i := pf.index(id); i >= 0 {
		return pf.proxies[i]
	}
	return nil
}

// kindProxies are the saved proxies of the kind the page shows, in order:
// those the page's list has rows for.
func (pf *proxyFields) kindProxies() []*config.ProxyEntry {
	var out []*config.ProxyEntry
	for _, e := range pf.proxies {
		if e.Kind == pf.kind {
			out = append(out, e)
		}
	}
	return out
}

// setKind chooses what the host connects through. The proxy chosen before
// was of the other kind, so the page chooses none of this one.
func (pf *proxyFields) setKind(k config.ProxyKind) {
	if pf.kind == k {
		return
	}
	pf.kind, pf.entry = k, ""
	if k != config.ProxyJump {
		pf.jump = ""
	}
}

// useEntry makes the host connect through the saved proxy with the ID,
// rather than go through a jump host.
func (pf *proxyFields) useEntry(id string) {
	pf.entry, pf.jump = id, ""
}

// useJump makes the host go through the saved host with the ID, rather
// than connect through a proxy: "" only gives up the jump host.
func (pf *proxyFields) useJump(id string) {
	pf.jump = id
	if id != "" {
		pf.entry = ""
	}
}

// proxyProblems adds to errs what is wrong with the Proxy page.
func (ed *connEditor) proxyProblems(errs map[editorField]string) {
	pf := &ed.proxy
	if pf.kind != config.ProxyJump {
		if config.IsProxyServer(pf.kind) && pf.proxy(pf.entry) == nil {
			errs[fieldProxyEntry] = locales.ErrProxyRequired()
		}
		return
	}
	j := pf.hosts.Host(pf.jump)
	if pf.jump == "" || j == nil || j.Deleted() || j.ID == ed.id {
		errs[fieldJumpHost] = locales.ErrJumpHostRequired()
		return
	}
	// The chain from here, as this host would be saved.
	self := &config.Host{ID: ed.id, Name: strings.TrimSpace(ed.name), Addr: strings.TrimSpace(ed.addr),
		Proxy: &config.ProxyConfig{Kind: config.ProxyJump, JumpHost: pf.jump}}
	if self.Title() == "" {
		self.Name = locales.NewConnection()
	}
	_, err := pf.hosts.JumpHosts(self)
	var loop *config.JumpLoopError
	var missing *config.JumpError
	switch {
	case errors.As(err, &loop):
		errs[fieldJumpHost] = locales.ErrJumpLoop(strings.Join(loop.Names, " → "))
	case errors.As(err, &missing):
		errs[fieldJumpHost] = locales.ErrJumpHostMissing(missing.From)
	}
}

// saveProxy saves the Proxy page into h: the saved proxy it chose, of its
// kind, or the jump host, or neither.
func (a *App) saveProxy(h *config.Host, pf *proxyFields) {
	// A new value, not the saved one changed: a session connecting reads
	// its copy of the host's.
	var p config.ProxyConfig
	if pf.saved != nil {
		p = *pf.saved
	}
	p.ID, p.Kind, p.Addr, p.Port, p.User, p.JumpHost = "", config.ProxyNone, "", 0, "", ""
	switch {
	case pf.kind == config.ProxyJump:
		if pf.jump == "" {
			h.Proxy = nil
			return
		}
		p.Kind, p.JumpHost = config.ProxyJump, pf.jump
	case !config.IsProxyServer(pf.kind):
		h.Proxy = nil
		return
	default:
		e := pf.proxy(pf.entry)
		if e == nil {
			h.Proxy = nil
			return
		}
		p.ID, p.Kind, p.Addr, p.Port, p.User = e.ID, e.Kind, e.Addr, e.Port, e.User
	}
	h.Proxy = &p
	pf.saved = &p
}

// saveProxyPasswords keeps the passwords the dialog wrote, under the
// proxies they belong to, and forgets those of the proxies the page
// removed.
func (a *App) saveProxyPasswords(pf *proxyFields) {
	for id := range pf.changed {
		a.saveProxyPassword(id, pf.passwords[id])
	}
	for _, id := range pf.removed {
		a.saveProxyPassword(id, "")
	}
}

// jumpChoice is a host the Proxy page offers as the jump host.
type jumpChoice struct {
	id, label string
}

// jumpChoices are the hosts that can be the jump host of the host with
// the ID: the others not deleted, in order, each labeled with its name
// and user@host, told apart when two read the same.
func jumpChoices(hosts *config.Store, id string) []jumpChoice {
	var out []jumpChoice
	seen := map[string]int{}
	for _, h := range hosts.Live() {
		if h.ID == id {
			continue
		}
		label := h.Target()
		if h.Name != "" {
			label = h.Name + " (" + label + ")"
		}
		if seen[label]++; seen[label] > 1 {
			label += " #" + strconv.Itoa(seen[label])
		}
		out = append(out, jumpChoice{h.ID, label})
	}
	return out
}

// proxyKindLabel is the kind of a saved proxy, as the page shows it.
func proxyKindLabel(k config.ProxyKind) string {
	switch k {
	case config.ProxySOCKS5:
		return "SOCKS5"
	case config.ProxyHTTP:
		return "HTTP"
	}
	return string(k)
}

// proxyKindHint says what connecting a way does.
func proxyKindHint(k config.ProxyKind) string {
	switch k {
	case config.ProxySOCKS5:
		return locales.ProxySOCKS5Hint()
	case config.ProxyHTTP:
		return locales.ProxyHTTPHint()
	case config.ProxyJump:
		return locales.ProxyJumpHint()
	}
	return locales.ProxyNoneHint()
}

// proxyColumns are the columns of the page's table of saved proxies.
func proxyColumns() []ui.TableColumn {
	return []ui.TableColumn{
		{ID: "current", Title: locales.ColProxyCurrent(), Width: 52, Align: ui.Center},
		{ID: "name", Title: locales.ColProxyName(), MinWidth: 110},
		{ID: "type", Title: locales.ColProxyType(), Width: 84},
		{ID: "host", Title: locales.ColProxyHost(), MinWidth: 100},
		{ID: "port", Title: locales.ColProxyPort(), Width: 64},
	}
}

// proxyPage is the editor's Proxy page.
func (a *App) proxyPage(c *ui.Context, ed *connEditor, errs map[editorField]string) {
	pf := &ed.proxy
	ui.Form(c, func() {
		ui.Field(c, locales.FieldProxyType(), func() {
			proxyKindChoice(c, pf)
		}).Description(proxyKindHint(pf.kind))
		switch {
		case pf.kind == config.ProxyJump:
			a.jumpField(c, ed, errs)
		case !config.IsProxyServer(pf.kind):
			// Nothing else: the host connects directly.
		default:
			ui.Field(c, locales.FieldProxyServer(), func() {
				ui.Column(c).Gap(8).Children(func() {
					proxyTable(c, pf)
					a.proxyButtons(c, pf)
				})
			}).Error(errs[fieldProxyEntry])
		}
	})
	ui.Modal(c, &pf.dialog, func() { a.proxyDialog(c, pf) })
}

// proxyKindChoice is the page's choice of what the host connects through,
// whose list shows for a proxy.
func proxyKindChoice(c *ui.Context, pf *proxyFields) {
	kinds := []config.ProxyKind{config.ProxyNone, config.ProxySOCKS5, config.ProxyHTTP, config.ProxyJump}
	labels := []string{locales.ProxyNoProxy(), proxyKindLabel(config.ProxySOCKS5),
		proxyKindLabel(config.ProxyHTTP), locales.ProxyJump()}
	chosen := labels[max(0, slices.Index(kinds, pf.kind))]
	if ui.Select(c, &chosen, labels).Width(200).Changed() {
		if i := slices.Index(labels, chosen); i >= 0 {
			pf.setKind(kinds[i])
		}
	}
}

// proxyTable is the table of the saved proxies of the kind chosen: the
// checked row is the one the host connects through.
func proxyTable(c *ui.Context, pf *proxyFields) {
	list := pf.kindProxies()
	pf.table.Label = func(i int) string {
		if i < 0 || i >= len(list) {
			return ""
		}
		return list[i].Title()
	}
	ui.Table(c, &pf.table, proxyColumns(), len(list), func(row, col int) {
		e := list[row]
		switch col {
		case 0:
			on := pf.entry == e.ID
			if ui.Checkbox(c, &on, "").Changed() {
				if on {
					pf.useEntry(e.ID)
				} else {
					pf.entry = ""
				}
			}
		case 1:
			ui.Text(c, e.Title()).SingleLine()
		case 2:
			ui.Text(c, proxyKindLabel(e.Kind)).SingleLine()
		case 3:
			ui.Text(c, e.Addr).SingleLine()
		case 4:
			ui.Text(c, strconv.Itoa(e.PortOrDefault())).SingleLine()
		}
	}).Height(168).Border(1, theme.Border).Radius(6)
}

// proxyButtons are the buttons that add a saved proxy, change the one
// chosen and remove it.
func (a *App) proxyButtons(c *ui.Context, pf *proxyFields) {
	chosen := pf.proxy(pf.entry)
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, locales.ProxyEdit()).Disabled(chosen == nil).Clicked() && chosen != nil {
			a.openProxyDialog(pf, pf.index(chosen.ID))
		}
		if ui.Button(c, locales.ProxyAdd()).Clicked() {
			a.openProxyDialog(pf, -1)
		}
		if ui.Button(c, locales.ProxyRemove()).Disabled(chosen == nil).Clicked() && chosen != nil {
			i := pf.index(chosen.ID)
			pf.removed = append(pf.removed, chosen.ID)
			pf.proxies = slices.Delete(pf.proxies, i, i+1)
			delete(pf.passwords, chosen.ID)
			delete(pf.changed, chosen.ID)
			pf.entry = ""
			// Nothing left of that kind to choose: the host connects
			// directly, rather than nowhere.
			if len(pf.kindProxies()) == 0 {
				pf.kind = config.ProxyNone
			}
		}
	})
}

// jumpField is the page's choice of a jump host: another saved host to log
// in to first, as ssh -J, which is the other way to reach this one.
func (a *App) jumpField(c *ui.Context, ed *connEditor, errs map[editorField]string) {
	pf := &ed.proxy
	choices := jumpChoices(pf.hosts, ed.id)
	note := locales.JumpHostNote()
	if len(choices) == 0 {
		note = locales.NoJumpHosts()
	}
	ui.Field(c, locales.FieldJumpHost(), func() {
		labels := []string{locales.ProxyNoProxy()}
		ids := []string{""}
		chosen := labels[0]
		for _, ch := range choices {
			labels = append(labels, ch.label)
			ids = append(ids, ch.id)
			if ch.id == pf.jump {
				chosen = ch.label
			}
		}
		if ui.Select(c, &chosen, labels).Width(320).Changed() {
			if i := slices.Index(labels, chosen); i >= 0 {
				pf.useJump(ids[i])
			}
		}
	}).Error(errs[fieldJumpHost]).Description(note)
}

// proxyForm is the dialog's fields, as typed.
type proxyForm struct {
	name, addr, port, user, password string
	kind                             config.ProxyKind // ProxySOCKS5 or ProxyHTTP
	// keep is the saved proxy edited, whose keys this build does not know
	// are kept.
	keep config.ProxyEntry
	// checked is set once OK was pressed, as the editor's.
	checked bool
}

// proxyField names a field of the dialog that can be wrong.
type proxyField int

const (
	fieldProxyAddr proxyField = iota
	fieldProxyPort
)

// openProxyDialog shows the dialog for the saved proxy at i in the page's
// list, or a new one of the kind the page shows for -1.
func (a *App) openProxyDialog(pf *proxyFields, i int) {
	e := config.ProxyEntry{Kind: pf.kind}
	if !config.IsProxyServer(e.Kind) {
		e.Kind = config.ProxySOCKS5
	}
	e.Port = config.DefaultProxyPort(e.Kind)
	if i >= 0 && i < len(pf.proxies) {
		e = *pf.proxies[i]
	}
	f := proxyForm{kind: e.Kind, name: e.Name, addr: e.Addr, user: e.User, keep: e, password: pf.passwords[e.ID]}
	if port := e.PortOrDefault(); port != 0 {
		f.port = strconv.Itoa(port)
	}
	pf.form, pf.editing, pf.dialog = f, i, true
	// The password may be the credential store's alone: the dialog shows it
	// once read, while it is still open on that proxy.
	if e.ID != "" && e.User != "" && !pf.asked[e.ID] {
		pf.asked[e.ID] = true
		a.readProxyPassword(e.ID, func() {
			if a.editor == nil || &a.editor.proxy != pf {
				return
			}
			pw := a.proxySecrets()[proxySecretKey(e.ID)]
			if pw == "" {
				return
			}
			pf.passwords[e.ID] = pw
			if pf.dialog && pf.editing == i && pf.form.password == "" {
				pf.form.password = pw
			}
		})
	}
}

// setKind chooses the kind, with its usual port while the port is empty
// or the last kind's.
func (f *proxyForm) setKind(k config.ProxyKind) {
	if p := strings.TrimSpace(f.port); p == "" || p == strconv.Itoa(config.DefaultProxyPort(f.kind)) {
		f.port = ""
		if port := config.DefaultProxyPort(k); port != 0 {
			f.port = strconv.Itoa(port)
		}
	}
	f.kind = k
}

// proxy is the saved proxy the form says, with the keys the one edited had
// that this build does not know.
func (f *proxyForm) proxy() config.ProxyEntry {
	e := f.keep
	e.Name, e.Addr, e.User = strings.TrimSpace(f.name), strings.TrimSpace(f.addr), strings.TrimSpace(f.user)
	e.Kind = f.kind
	e.Port, _ = strconv.Atoi(strings.TrimSpace(f.port))
	return e
}

// problems are what is wrong with the dialog's fields, in the language
// shown.
func (pf *proxyFields) problems() map[proxyField]string {
	f := &pf.form
	errs := map[proxyField]string{}
	if addr := strings.TrimSpace(f.addr); addr == "" {
		errs[fieldProxyAddr] = locales.ErrHostRequired()
	} else if !validHost(addr) {
		errs[fieldProxyAddr] = locales.ErrListenAddrInvalid()
	}
	if _, ok := validPort(f.port); !ok {
		errs[fieldProxyPort] = locales.ErrPortInvalid()
	}
	return errs
}

// saveDialog puts the dialog's saved proxy in the list, with its password,
// and makes the host connect through it, unless something is wrong with
// it.
func (pf *proxyFields) saveDialog() {
	pf.form.checked = true
	if len(pf.problems()) > 0 {
		return
	}
	e := pf.form.proxy()
	if i := pf.editing; i >= 0 && i < len(pf.proxies) {
		e.ID = pf.proxies[i].ID
		pf.proxies[i] = &e
	} else {
		e.ID = config.NewProxyID()
		pf.proxies = append(pf.proxies, &e)
	}
	// What the dialog shows is the proxy's password: a proxy without a
	// user has none. Only one that changed is saved, so that one the
	// credential store has not shown yet is not forgotten, and one that
	// stays as it was is not written again.
	pw := ""
	if e.User != "" {
		pw = pf.form.password
	}
	if pw != pf.passwords[e.ID] {
		pf.passwords[e.ID], pf.changed[e.ID] = pw, true
	}
	// The page shows the proxy the dialog made, of its kind.
	pf.kind, pf.entry, pf.jump = e.Kind, e.ID, ""
	pf.dialog = false
}

// proxyDialog is the dialog that adds or changes a saved proxy, with its
// password, which is the proxy's. Enter is OK, and Escape, as a click
// outside, Cancel.
func (a *App) proxyDialog(c *ui.Context, pf *proxyFields) {
	f := &pf.form
	var errs map[proxyField]string
	if f.checked {
		errs = pf.problems()
	}
	// Enter where no field takes it, and in a text input, which takes it as
	// submitting: the editor's Enter waits while the dialog shows.
	submit := c.Shortcut(0, ui.KeyEnter)
	input := func(e ui.Element) ui.Element {
		submit = e.Submitted() || submit
		return e
	}
	title := locales.ProxyNew()
	if pf.editing >= 0 {
		title = locales.ProxyChange()
	}
	ui.Column(c).Width(440).Gap(10).Children(func() {
		ui.Text(c, title).FontSize(15).Bold()
		ui.Form(c, func() {
			ui.Field(c, locales.FieldProxyName(), func() {
				input(ui.TextInput(c, &f.name).Placeholder(locales.ProxyNamePlaceholder()).AutoFocus())
			})
			ui.Field(c, locales.FieldProxyType(), func() {
				kinds := []config.ProxyKind{config.ProxySOCKS5, config.ProxyHTTP}
				labels := []string{proxyKindLabel(config.ProxySOCKS5), proxyKindLabel(config.ProxyHTTP)}
				chosen := labels[max(0, slices.Index(kinds, f.kind))]
				if ui.Select(c, &chosen, labels).Width(160).Changed() {
					f.setKind(kinds[max(0, slices.Index(labels, chosen))])
				}
			}).Description(proxyKindHint(f.kind))
			ui.Field(c, locales.FieldHost(), func() {
				input(ui.TextInput(c, &f.addr).Placeholder(locales.FieldHostPlaceholder()))
			}).Error(errs[fieldProxyAddr])
			ui.Field(c, locales.FieldPort(), func() {
				input(ui.TextInput(c, &f.port).Width(90))
			}).Error(errs[fieldProxyPort])
			ui.Field(c, locales.FieldUser(), func() {
				input(ui.TextInput(c, &f.user).Placeholder(locales.ProxyUserPlaceholder()))
			})
			ui.Field(c, locales.FieldPassword(), func() {
				input(ui.TextInput(c, &f.password).Password())
			}).Description(a.rememberNote(true))
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, locales.Cancel()).Clicked() {
				pf.dialog = false
			}
			if ui.PrimaryButton(c, locales.Ok()).Clicked() {
				submit = true
			}
		})
	})
	if submit {
		pf.saveDialog()
	}
}
