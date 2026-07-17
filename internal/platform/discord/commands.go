package discord

func GlobalCommands() []ApplicationCommand {
	baseContexts := []int{
		InteractionContextGuild,
		InteractionContextBotDM,
		InteractionContextPrivateChannel,
	}
	guildInstall := []int{IntegrationTypeGuildInstall}

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
			IntegrationTypes: guildInstall,
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
			IntegrationTypes: guildInstall,
			Contexts:         baseContexts,
		},
		{
			Name:             CommandExplain,
			Type:             ApplicationCommandTypeMessage,
			IntegrationTypes: guildInstall,
			Contexts:         baseContexts,
		},
		{
			Name: "settings", Type: ApplicationCommandTypeChatInput,
			Description: "Настройки бота (только owner)", IntegrationTypes: guildInstall, Contexts: baseContexts,
			Options: []ApplicationCommandSchemaOption{{Type: ApplicationCommandOptionTypeString, Name: "command", Description: "например: set primary model gpt-4.1"}},
		},
	}
}
