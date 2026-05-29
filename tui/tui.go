package tui

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/peterjohnbishop/centra-chatter/storage"
	"golang.org/x/crypto/ssh"
)

var (
	baseStyle    = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).Padding(0, 1)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	blurredStyle = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	focusedStyle = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
)

const chatBanner = `▄▄▄▄▄▄ ▄▄▄▄▄ ▄▄▄▄  ▄▄   ▄▄ ▄▄ ▄▄  ▄▄  ▄▄▄  ▄▄       ▄▄▄▄ ▄▄ ▄▄  ▄▄▄ ▄▄▄▄▄▄ ▄▄▄▄▄▄ ▄▄▄▄▄ ▄▄▄▄  
 ██   ██▄▄  ██▄█▄ ██▀▄▀██ ██ ███▄██ ██▀██ ██  ▄▄▄ ██▀▀▀ ██▄██ ██▀██  ██     ██   ██▄▄  ██▄█▄ 
 ██   ██▄▄▄ ██ ██ ██  ██ ██ ██ ▀██ ██▀██ ██▄▄▄   ▀████ ██ ██ ██▀██  ██     ██   ██▄▄▄ ██ ██`

type tickMsg time.Time

func doTick() tea.Cmd {
	return tea.Tick(time.Second*1, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type tuiModel struct {
	db              *storage.Storage
	isAdmin         bool
	isAuthenticated bool
	username        string
	publicKey       ssh.PublicKey
	table           table.Model
	inputs          []textinput.Model
	focus           int
	focusPane       int // 0 = Admin Table, 1 = App View
	message         string
	viewport        viewport.Model
	textInput       textinput.Model
	messages        []string
	err             error
}

func InitialModel(db *storage.Storage, isAdmin bool, isAuthenticated bool, pubKey ssh.PublicKey, username string) *tuiModel {

	ti := textinput.New()
	ti.SetWidth(50)
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 256
	if !isAdmin {
		ti.Focus()
	}

	vp := viewport.New(
		viewport.WithWidth(50),
		viewport.WithHeight(10),
	)
	vp.SetContent("Welcome to Central Chatter!\n")

	m := &tuiModel{
		db:              db,
		isAdmin:         isAdmin,
		isAuthenticated: isAuthenticated,
		username:        username,
		publicKey:       pubKey,
		focusPane:       0,
		textInput:       ti,
		viewport:        vp,
		messages:        []string{"System: Welcome to Central Chatter!"},
	}

	if isAdmin {
		columns := []table.Column{
			{Title: "Username", Width: 15},
			{Title: "Online", Width: 8},
			{Title: "Status", Width: 12},
			{Title: "Role", Width: 8},
			{Title: "Message", Width: 30},
			{Title: "Date", Width: 12},
		}

		t := table.New(
			table.WithColumns(columns),
			table.WithFocused(true),
			table.WithHeight(8),
		)
		s := table.DefaultStyles()
		s.Header = s.Header.BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).Bold(true)
		s.Selected = s.Selected.Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(false)
		t.SetStyles(s)

		m.table = t
		m.refreshTable()

	} else if !isAuthenticated {
		m.inputs = make([]textinput.Model, 2)

		m.inputs[0] = textinput.New()
		m.inputs[0].Placeholder = "Desired Username"
		m.inputs[0].SetValue(username)
		m.inputs[0].SetWidth(30)
		m.inputs[0].CharLimit = 32
		m.inputs[0].Focus()

		m.inputs[1] = textinput.New()
		m.inputs[1].Placeholder = "Why do you need access?"
		m.inputs[1].SetWidth(50)
		m.inputs[1].CharLimit = 100
	}

	return m
}

func (m *tuiModel) refreshTable() {
	var rows []table.Row

	requests, err := m.db.GetAccessRequests()
	if err == nil {
		for _, r := range requests {
			username := r[1]
			message := r[3]
			createdAt := r[4]

			if len(createdAt) > 10 {
				createdAt = createdAt[:10]
			}

			rows = append(rows, table.Row{username, "-", "* Pending", "-", message, createdAt})
		}
	}

	users, err := m.db.GetAllUsers()
	if err == nil {
		for _, u := range users {
			username := u[0]
			isOnline := u[1]
			status := u[2]
			role := u[3]

			rows = append(rows, table.Row{username, isOnline, status, role, "", ""})
		}
	}

	m.table.SetRows(rows)
}

func (m *tuiModel) Init() tea.Cmd {
	if m.isAdmin {
		return doTick()
	}
	return tea.Batch(textinput.Blink, doTick())
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.isAdmin && m.focusPane == 1 {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.isAuthenticated && !m.isAdmin {
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		cmds = append(cmds, cmd)
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	} else if !m.isAuthenticated {
		for i := range m.inputs {
			var cmd tea.Cmd
			m.inputs[i], cmd = m.inputs[i].Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	if m.isAdmin {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tickMsg:
		if m.isAdmin {
			m.refreshTable()
		}

		if !m.isAuthenticated && m.publicKey != nil {
			if m.db.ValidatePublicKey(m.username, m.publicKey) {
				m.isAuthenticated = true
			}
		}

		if m.isAuthenticated || m.isAdmin {
			recentMessages, err := m.db.GetRecentMessages(100)
			if err == nil {
				if len(recentMessages) != len(m.messages) {
					m.messages = recentMessages
					m.viewport.SetContent(m.formatMessages())
					m.viewport.GotoBottom()
				}
			}
		}

		cmds = append(cmds, doTick())

	case tea.WindowSizeMsg:
		chatWidth := msg.Width - 4
		if m.isAdmin {
			m.table.SetWidth(chatWidth)
			m.viewport.SetHeight(msg.Height - 24)
		} else {
			m.viewport.SetHeight(msg.Height - 10)
		}
		m.viewport.SetWidth(chatWidth)
		m.textInput.SetWidth(chatWidth - 4)

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
		if m.isAdmin {

			if msg.String() == "tab" || msg.String() == "shift+tab" {
				if m.focusPane == 0 {
					m.focusPane = 1
					m.table.Blur()
					m.textInput.Focus()
				} else {
					m.focusPane = 0
					m.table.Focus()
					m.textInput.Blur()
				}
				return m, tea.Batch(cmds...)
			}

			if m.focusPane == 0 {
				switch msg.String() {
				case "A", "a":
					if selected := m.table.SelectedRow(); selected != nil {
						username := selected[0]
						status := selected[2]

						if status == "* Pending" {
							m.db.ApproveRequest(username)
						} else {
							m.db.ToggleApproval(username)
						}
						m.refreshTable()
					}
					return m, nil
				case "P", "p":
					if selected := m.table.SelectedRow(); selected != nil {
						m.db.PromoteAdmin(selected[0])
						m.refreshTable()
					}
					return m, nil
				case "D", "d":
					if selected := m.table.SelectedRow(); selected != nil {
						m.db.DemoteAdmin(selected[0])
						m.refreshTable()
					}
					return m, nil
				case "X", "x":
					if selected := m.table.SelectedRow(); selected != nil {
						m.db.DeleteUser(selected[0])
						m.refreshTable()
					}
					return m, nil
				}
			}
		}

		isChatActive := (m.isAuthenticated && !m.isAdmin) || (m.isAdmin && m.focusPane == 1)

		if isChatActive {
			if msg.String() == "enter" && m.textInput.Value() != "" {
				msgText := m.textInput.Value()
				m.textInput.Reset()

				// Save to SQLite database
				m.db.SaveMessage(m.username, msgText)

				// Force an immediate refresh
				recentMessages, err := m.db.GetRecentMessages(100)
				if err == nil {
					m.messages = recentMessages
					m.viewport.SetContent(m.formatMessages())
					m.viewport.GotoBottom()
				}
			}
		}

		if !m.isAuthenticated && !m.isAdmin {
			switch msg.String() {
			case "tab", "shift+tab", "up", "down":
				s := msg.String()
				if s == "up" || s == "shift+tab" {
					m.focus--
				} else {
					m.focus++
				}
				if m.focus > len(m.inputs)-1 {
					m.focus = 0
				} else if m.focus < 0 {
					m.focus = len(m.inputs) - 1
				}

				for i := 0; i <= len(m.inputs)-1; i++ {
					if i == m.focus {
						cmds = append(cmds, m.inputs[i].Focus())
						continue
					}
					m.inputs[i].Blur()
				}
				return m, tea.Batch(cmds...)

			case "enter":
				username := m.inputs[0].Value()
				msgTxt := m.inputs[1].Value()

				var pubKeyStr string
				if m.publicKey != nil {
					pubKeyBytes := ssh.MarshalAuthorizedKey(m.publicKey)
					pubKeyStr = string(pubKeyBytes)
				}

				err := m.db.SubmitRequest(username, pubKeyStr, msgTxt)
				if err != nil {
					m.err = err
					m.message = err.Error()
				} else {
					m.message = "Request submitted successfully. Waiting for admin approval."
					m.err = nil
				}
				return m, nil
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *tuiModel) formatMessages() string {
	var b strings.Builder

	chatWidth := m.viewport.Width()
	if chatWidth > 2 {
		chatWidth -= 2
	}

	localStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Align(lipgloss.Right).Width(chatWidth)
	remoteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Align(lipgloss.Left).Width(chatWidth)
	systemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Align(lipgloss.Center).Width(chatWidth)
	deletedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Align(lipgloss.Left).Width(chatWidth)

	localPrefix := m.username + ":"

	for _, msg := range m.messages {
		if strings.HasPrefix(msg, localPrefix) {
			b.WriteString(localStyle.Render(msg))
			b.WriteString("\n")
		} else if strings.HasPrefix(msg, "System:") {
			b.WriteString(systemStyle.Render(msg))
			b.WriteString("\n")
		} else if strings.HasPrefix(msg, "DELETED:") {
			b.WriteString(deletedStyle.Render(msg))
			b.WriteString("\n")
		} else {
			b.WriteString(remoteStyle.Render(msg))
			b.WriteString("\n")
		}
	}

	rendered := b.String()

	rendered = strings.TrimSuffix(rendered, "\n")

	contentHeight := lipgloss.Height(rendered)
	vpHeight := m.viewport.Height()

	if contentHeight < vpHeight {
		padding := strings.Repeat("\n", vpHeight-contentHeight)
		rendered = padding + rendered
	}

	return rendered
}

// Helper to quickly render the messaging stack
func (m *tuiModel) chatView() string {
	bannerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Align(lipgloss.Center).
		Width(m.viewport.Width()).
		MarginBottom(1)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		bannerStyle.Render(chatBanner),
		m.viewport.View(),
		lipgloss.NewStyle().MarginTop(1).Render(m.textInput.View()),
	)
}

func (m *tuiModel) View() tea.View {
	if m.isAdmin {
		tableStyle := focusedStyle
		appStyle := blurredStyle
		if m.focusPane == 1 {
			tableStyle = blurredStyle
			appStyle = focusedStyle
		}

		footer := "\n\n[A] Approve/Revoke • [P] Promote • [D] Demote • [X] Delete • [Tab] Switch Focus • [Esc] Quit"
		tableSection := tableStyle.Render(m.table.View() + footer)

		// Inject the new dynamic chat UI
		appSection := appStyle.Render(m.chatView())

		v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, tableSection, appSection))
		v.AltScreen = true
		return v
	}

	if m.isAuthenticated {
		v := tea.NewView(baseStyle.Render(m.chatView()))
		v.AltScreen = true
		return v
	}

	var b strings.Builder
	b.WriteString("Request System Access\n\n")

	for i := range m.inputs {
		b.WriteString(m.inputs[i].View())
		b.WriteRune('\n')
	}

	b.WriteString("\n[Tab] Next Field • [Enter] Submit • [Esc] Quit\n")

	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("Error: " + m.err.Error()))
	}
	if m.message != "" {
		b.WriteString("\n")
		b.WriteString(successStyle.Render(m.message))
	}

	v := tea.NewView(baseStyle.Render(b.String()))
	v.AltScreen = true
	return v
}
