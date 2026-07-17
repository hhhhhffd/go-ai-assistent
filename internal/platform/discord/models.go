package discord

import "encoding/json"

const (
	InteractionTypePing               = 1
	InteractionTypeApplicationCommand = 2
)

const (
	InteractionCallbackTypePong                             = 1
	InteractionCallbackTypeChannelMessageWithSource         = 4
	InteractionCallbackTypeDeferredChannelMessageWithSource = 5
)

const (
	ApplicationCommandTypeChatInput = 1
	ApplicationCommandTypeMessage   = 3
)

const (
	ApplicationCommandOptionTypeString  = 3
	ApplicationCommandOptionTypeBoolean = 5
)

const (
	IntegrationTypeGuildInstall = 0
	IntegrationTypeUserInstall  = 1
)

const (
	InteractionContextGuild          = 0
	InteractionContextBotDM          = 1
	InteractionContextPrivateChannel = 2
)

const (
	MessageFlagEphemeral = 1 << 6
)

const (
	CommandAsk     = "ask"
	CommandDeep    = "deep"
	CommandExplain = "Объяснить"
)

const (
	DiscordMessageLimit = 2000
)

type Interaction struct {
	ID            string                  `json:"id"`
	ApplicationID string                  `json:"application_id"`
	Type          int                     `json:"type"`
	Token         string                  `json:"token"`
	Context       int                     `json:"context,omitempty"`
	GuildID       string                  `json:"guild_id,omitempty"`
	ChannelID     string                  `json:"channel_id,omitempty"`
	Member        *InteractionMember      `json:"member,omitempty"`
	User          *User                   `json:"user,omitempty"`
	Data          *ApplicationCommandData `json:"data,omitempty"`
}

type InteractionMember struct {
	User *User `json:"user,omitempty"`
}

type User struct {
	ID string `json:"id"`
}

type ApplicationCommandData struct {
	ID       string                     `json:"id"`
	Type     int                        `json:"type"`
	Name     string                     `json:"name"`
	Options  []ApplicationCommandOption `json:"options,omitempty"`
	TargetID string                     `json:"target_id,omitempty"`
	Resolved *ResolvedData              `json:"resolved,omitempty"`
}

type ApplicationCommandOption struct {
	Name  string          `json:"name"`
	Type  int             `json:"type"`
	Value json.RawMessage `json:"value,omitempty"`
}

type ResolvedData struct {
	Messages map[string]ResolvedMessage `json:"messages,omitempty"`
}

type ResolvedMessage struct {
	Content string `json:"content"`
}

type InteractionResponse struct {
	Type int                      `json:"type"`
	Data *InteractionCallbackData `json:"data,omitempty"`
}

type InteractionCallbackData struct {
	Content         string           `json:"content,omitempty"`
	Flags           int              `json:"flags,omitempty"`
	AllowedMentions *AllowedMentions `json:"allowed_mentions,omitempty"`
}

type AllowedMentions struct {
	Parse []string `json:"parse"`
}

type ApplicationCommand struct {
	Name             string                           `json:"name"`
	Type             int                              `json:"type,omitempty"`
	Description      string                           `json:"description,omitempty"`
	Options          []ApplicationCommandSchemaOption `json:"options,omitempty"`
	IntegrationTypes []int                            `json:"integration_types,omitempty"`
	Contexts         []int                            `json:"contexts,omitempty"`
}

type ApplicationCommandSchemaOption struct {
	Type        int    `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

type EditWebhookMessageRequest struct {
	Content         string           `json:"content"`
	AllowedMentions *AllowedMentions `json:"allowed_mentions,omitempty"`
}

type FollowupMessageRequest struct {
	Content         string           `json:"content"`
	Flags           int              `json:"flags,omitempty"`
	AllowedMentions *AllowedMentions `json:"allowed_mentions,omitempty"`
}
