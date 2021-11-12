package main

import (
	"context"
	"log"

	"github.com/digitalocean/godo"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func bot(ctx context.Context, client *godo.Client, cfg BotConfig) error {
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return err
	}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		chatID := update.Message.Chat.ID

		if chatID != cfg.TargetChat {
			bot.Send(tgbotapi.NewMessage(chatID, "I'm not permitted to talk with you"))
			continue
		}
	}

	return nil
}
