package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type Plugin struct {
	plugin.MattermostPlugin
	configurationLock sync.RWMutex
	configuration     *configuration
	botUserID         string
}

type configuration struct {
	GoogleOAuthClientID       string `json:"GoogleOAuthClientID"`
	GoogleOAuthClientSecret   string `json:"GoogleOAuthClientSecret"`
	EncryptionKey             string `json:"EncryptionKey"`
	ServiceAccountCredentials string `json:"ServiceAccountCredentials"`
}

func (c *configuration) Clone() *configuration {
	var clone = *c
	return &clone
}

func (c *configuration) IsValid() error {
	if c.GoogleOAuthClientID == "" {
		return errors.New("Google OAuth Client ID is not configured")
	}
	if c.GoogleOAuthClientSecret == "" {
		return errors.New("Google OAuth Client Secret is not configured")
	}
	return nil
}

func (p *Plugin) getConfiguration() *configuration {
	p.configurationLock.RLock()
	defer p.configurationLock.RUnlock()

	if p.configuration == nil {
		return &configuration{}
	}

	return p.configuration
}

func (p *Plugin) setConfiguration(configuration *configuration) {
	p.configurationLock.Lock()
	defer p.configurationLock.Unlock()

	if configuration != nil && p.configuration == configuration {
		panic("setConfiguration called with the existing configuration")
	}

	p.configuration = configuration
}

func (p *Plugin) ensureBot(bot *model.Bot) (string, error) {
	// Check if bot already exists
	existingBot, appErr := p.API.GetUserByUsername(bot.Username)
	if appErr == nil && existingBot != nil {
		return existingBot.Id, nil
	}

	// Create the bot
	createdBot, appErr := p.API.CreateBot(bot)
	if appErr != nil {
		return "", errors.Wrap(appErr, "failed to create bot")
	}

	return createdBot.UserId, nil
}

func (p *Plugin) OnActivate() error {
	bot := &model.Bot{
		Username:    "google-drive",
		DisplayName: "Google Drive",
		Description: "Bot for Google Drive plugin",
	}

	botUserID, err := p.ensureBot(bot)
	if err != nil {
		return errors.Wrap(err, "failed to ensure bot user")
	}
	p.botUserID = botUserID

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "drive",
		DisplayName:      "Google Drive",
		Description:      "Integration with Google Drive",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: connect, disconnect, create, notifications, help",
		AutoCompleteHint: "[command]",
		AutocompleteData: p.getAutocompleteData(),
	}); err != nil {
		return errors.Wrap(err, "failed to register command")
	}

	return nil
}

func (p *Plugin) getAutocompleteData() *model.AutocompleteData {
	data := model.NewAutocompleteData("drive", "[command]", "Available commands: connect, disconnect, create, notifications, help")

	connect := model.NewAutocompleteData("connect", "", "Connect your Google account")
	data.AddCommand(connect)

	disconnect := model.NewAutocompleteData("disconnect", "", "Disconnect your Google account")
	data.AddCommand(disconnect)

	create := model.NewAutocompleteData("create", "[docs|slides|sheets]", "Create a new Google Drive file")
	create.AddStaticListArgument("File type", true, []model.AutocompleteListItem{
		{Item: "docs", HelpText: "Create a new Google Docs document"},
		{Item: "slides", HelpText: "Create a new Google Slides presentation"},
		{Item: "sheets", HelpText: "Create a new Google Sheets spreadsheet"},
	})
	data.AddCommand(create)

	notifications := model.NewAutocompleteData("notifications", "[start|stop]", "Manage Google Drive notifications")
	notifications.AddStaticListArgument("Action", true, []model.AutocompleteListItem{
		{Item: "start", HelpText: "Start receiving notifications"},
		{Item: "stop", HelpText: "Stop receiving notifications"},
	})
	data.AddCommand(notifications)

	help := model.NewAutocompleteData("help", "", "Show help information")
	data.AddCommand(help)

	return data
}

func (p *Plugin) OnConfigurationChange() error {
	var configuration = new(configuration)

	if err := p.API.LoadPluginConfiguration(configuration); err != nil {
		return errors.Wrap(err, "failed to load plugin configuration")
	}

	p.setConfiguration(configuration)

	return nil
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	config := p.getConfiguration()
	if err := config.IsValid(); err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Plugin is not configured: %s", err.Error()),
		}, nil
	}

	return p.executeCommand(c, args)
}

func (p *Plugin) executeCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	split := splitArgs(args.Command)
	if len(split) < 2 {
		return p.handleHelp(args)
	}

	command := split[1]

	switch command {
	case "connect":
		return p.handleConnect(args)
	case "disconnect":
		return p.handleDisconnect(args)
	case "create":
		return p.handleCreate(args, split)
	case "notifications":
		return p.handleNotifications(args, split)
	case "help":
		return p.handleHelp(args)
	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Unknown command: %s. Use `/drive help` for available commands.", command),
		}, nil
	}
}

func (p *Plugin) getOAuthConfig() *oauth2.Config {
	config := p.getConfiguration()
	return &oauth2.Config{
		ClientID:     config.GoogleOAuthClientID,
		ClientSecret: config.GoogleOAuthClientSecret,
		Scopes: []string{
			"https://www.googleapis.com/auth/drive",
			"https://www.googleapis.com/auth/drive.file",
			"https://www.googleapis.com/auth/documents",
			"https://www.googleapis.com/auth/spreadsheets",
			"https://www.googleapis.com/auth/presentations",
		},
		Endpoint: google.Endpoint,
		RedirectURL: fmt.Sprintf("%s/plugins/com.mattermost.google-drive/oauth/complete",
			*p.API.GetConfig().ServiceSettings.SiteURL),
	}
}

func (p *Plugin) handleConnect(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	oauthConfig := p.getOAuthConfig()

	state := fmt.Sprintf("%s_%s", args.UserId, model.NewId())
	if err := p.storeOAuthState(args.UserId, state); err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Failed to initiate OAuth flow. Please try again.",
		}, nil
	}

	authURL := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf("[Click here to connect your Google account](%s)", authURL),
	}, nil
}

func (p *Plugin) handleDisconnect(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	if err := p.disconnectUser(args.UserId); err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Failed to disconnect your account. Please try again.",
		}, nil
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         "Your Google account has been disconnected.",
	}, nil
}

func (p *Plugin) handleCreate(args *model.CommandArgs, split []string) (*model.CommandResponse, *model.AppError) {
	if len(split) < 3 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Please specify the file type: `/drive create [docs|slides|sheets]`",
		}, nil
	}

	fileType := split[2]
	if fileType != "docs" && fileType != "slides" && fileType != "sheets" {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Invalid file type. Use one of: docs, slides, sheets",
		}, nil
	}

	token, err := p.getTokenForUser(args.UserId)
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "You need to connect your Google account first. Use `/drive connect`.",
		}, nil
	}

	fileURL, err := p.createGoogleFile(token, fileType, "Untitled")
	if err != nil {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         fmt.Sprintf("Failed to create file: %s", err.Error()),
		}, nil
	}

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf("File created successfully: %s", fileURL),
	}, nil
}

func (p *Plugin) handleNotifications(args *model.CommandArgs, split []string) (*model.CommandResponse, *model.AppError) {
	if len(split) < 3 {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Please specify the action: `/drive notifications [start|stop]`",
		}, nil
	}

	action := split[2]

	switch action {
	case "start":
		if err := p.enableNotifications(args.UserId); err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Failed to enable notifications. Please try again.",
			}, nil
		}
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Google Drive notifications have been enabled.",
		}, nil

	case "stop":
		if err := p.disableNotifications(args.UserId); err != nil {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         "Failed to disable notifications. Please try again.",
			}, nil
		}
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Google Drive notifications have been disabled.",
		}, nil

	default:
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Invalid action. Use: start or stop",
		}, nil
	}
}

func (p *Plugin) handleHelp(args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	helpText := `### Google Drive Plugin Commands

- **` + "`/drive connect`" + `** - Connect your Google account
- **` + "`/drive disconnect`" + `** - Disconnect your Google account
- **` + "`/drive create [docs|slides|sheets]`" + `** - Create a new Google Drive file
- **` + "`/drive notifications [start|stop]`" + `** - Enable or disable notifications
- **` + "`/drive help`" + `** - Show this help message

For more information, visit the [Google Drive Plugin documentation](https://github.com/mattermost/mattermost-app-google-drive).`

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         helpText,
	}, nil
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/oauth/complete":
		p.handleOAuthComplete(w, r)
	case "/webhook":
		p.handleWebhook(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *Plugin) handleOAuthComplete(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	userID, err := p.validateOAuthState(state)
	if err != nil {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	oauthConfig := p.getOAuthConfig()

	token, err := oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}

	if err := p.storeToken(userID, token); err != nil {
		http.Error(w, "Failed to store token", http.StatusInternalServerError)
		return
	}

	p.API.SendEphemeralPost(userID, &model.Post{
		UserId:    p.botUserID,
		ChannelId: p.getDirectChannel(userID),
		Message:   "Your Google account has been connected successfully!",
	})

	html := `<!DOCTYPE html>
<html>
<head>
    <title>Google Drive Connected</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background-color: #f4f8fb; }
        .container { text-align: center; padding: 40px; background: white; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
        h1 { color: #166de0; }
        p { color: #3d3c40; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Connected!</h1>
        <p>Your Google account has been connected to Mattermost.</p>
        <p>You can close this window and return to Mattermost.</p>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	if _, err := w.Write([]byte(html)); err != nil {
		p.API.LogError("Failed to write OAuth complete response", "error", err.Error())
	}
}

func (p *Plugin) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (p *Plugin) getDirectChannel(userID string) string {
	channel, err := p.API.GetDirectChannel(userID, p.botUserID)
	if err != nil {
		return ""
	}
	return channel.Id
}

func (p *Plugin) storeOAuthState(userID, state string) error {
	data, _ := json.Marshal(map[string]string{"user_id": userID})
	if appErr := p.API.KVSetWithExpiry(fmt.Sprintf("oauth_state_%s", state), data, 300); appErr != nil {
		return errors.Wrap(appErr, "failed to store OAuth state")
	}
	return nil
}

func (p *Plugin) validateOAuthState(state string) (string, error) {
	data, appErr := p.API.KVGet(fmt.Sprintf("oauth_state_%s", state))
	if appErr != nil || data == nil {
		return "", errors.New("invalid state")
	}

	var stateData map[string]string
	if err := json.Unmarshal(data, &stateData); err != nil {
		return "", err
	}

	// Delete state after validation (ignore error as state cleanup is best-effort)
	_ = p.API.KVDelete(fmt.Sprintf("oauth_state_%s", state))

	return stateData["user_id"], nil
}

func (p *Plugin) storeToken(userID string, token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}

	if appErr := p.API.KVSet(fmt.Sprintf("google_token_%s", userID), data); appErr != nil {
		return errors.Wrap(appErr, "failed to store token")
	}
	return nil
}

func (p *Plugin) getTokenForUser(userID string) (*oauth2.Token, error) {
	data, appErr := p.API.KVGet(fmt.Sprintf("google_token_%s", userID))
	if appErr != nil || data == nil {
		return nil, errors.New("no token found")
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}

	return &token, nil
}

func (p *Plugin) disconnectUser(userID string) error {
	// Delete user data (ignore errors as cleanup is best-effort)
	_ = p.API.KVDelete(fmt.Sprintf("google_token_%s", userID))
	_ = p.API.KVDelete(fmt.Sprintf("notifications_%s", userID))
	return nil
}

type driveFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
}

func (p *Plugin) createGoogleFile(token *oauth2.Token, fileType, title string) (string, error) {
	var mimeType string
	var urlPrefix string
	switch fileType {
	case "docs":
		mimeType = "application/vnd.google-apps.document"
		urlPrefix = "https://docs.google.com/document/d/"
	case "slides":
		mimeType = "application/vnd.google-apps.presentation"
		urlPrefix = "https://docs.google.com/presentation/d/"
	case "sheets":
		mimeType = "application/vnd.google-apps.spreadsheet"
		urlPrefix = "https://docs.google.com/spreadsheets/d/"
	default:
		return "", errors.New("invalid file type")
	}

	reqBody := map[string]string{
		"name":     title,
		"mimeType": mimeType,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://www.googleapis.com/drive/v3/files", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create file: %s", string(body))
	}

	var file driveFile
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return "", err
	}

	return urlPrefix + file.ID + "/edit", nil
}

func (p *Plugin) enableNotifications(userID string) error {
	if appErr := p.API.KVSet(fmt.Sprintf("notifications_%s", userID), []byte("enabled")); appErr != nil {
		return errors.Wrap(appErr, "failed to enable notifications")
	}
	return nil
}

func (p *Plugin) disableNotifications(userID string) error {
	// Ignore error as disabling is best-effort
	_ = p.API.KVDelete(fmt.Sprintf("notifications_%s", userID))
	return nil
}

func splitArgs(command string) []string {
	var args []string
	var current string
	inQuotes := false

	for _, char := range command {
		if char == '"' {
			inQuotes = !inQuotes
		} else if char == ' ' && !inQuotes {
			if current != "" {
				args = append(args, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		args = append(args, current)
	}

	return args
}

func main() {
	plugin.ClientMain(&Plugin{})
}
