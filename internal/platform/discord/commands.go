package discord

func GlobalCommands() []ApplicationCommand {
	baseContexts := []int{
		InteractionContextGuild,
		InteractionContextBotDM,
		InteractionContextPrivateChannel,
	}
	userInstall := []int{IntegrationTypeUserInstall}

	return []ApplicationCommand{
		{
			Name:        CommandAsk,
			Type:        ApplicationCommandTypeChatInput,
			Description: "Задать вопрос ассистенту",
			Options: []ApplicationCommandSchemaOption{
				{
					Type:        ApplicationCommandOptionTypeString,
					Name:        "question",
					Description: "Вопрос для ассистента",
					Required:    true,
				},
				{
					Type:        ApplicationCommandOptionTypeBoolean,
					Name:        "public",
					Description: "Показать ответ публично (по умолчанию приватно)",
				},
			},
			IntegrationTypes: userInstall,
			Contexts:         baseContexts,
		},
		{
			Name:        CommandDeep,
			Type:        ApplicationCommandTypeChatInput,
			Description: "Глубокий разбор вопроса",
			Options: []ApplicationCommandSchemaOption{
				{
					Type:        ApplicationCommandOptionTypeString,
					Name:        "question",
					Description: "Вопрос для глубокого анализа",
					Required:    true,
				},
				{
					Type:        ApplicationCommandOptionTypeBoolean,
					Name:        "public",
					Description: "Показать ответ публично (по умолчанию приватно)",
				},
			},
			IntegrationTypes: userInstall,
			Contexts:         baseContexts,
		},
		{
			Name:             CommandExplain,
			Type:             ApplicationCommandTypeMessage,
			IntegrationTypes: userInstall,
			Contexts:         baseContexts,
		},
	}
}
