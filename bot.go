package main

import (
	"context"
	"fmt"
	"log"

	"github.com/digitalocean/godo"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Action func() error

func runAction(action Action, ch chan error) {
	ch <- action()
}

func checkCmdValid(cmd string, allowed []string, suffix string) bool {
	for _, c := range allowed {
		if cmd == c || cmd == c+"@"+suffix {
			return true
		}
	}

	return false
}

func bot(ctx context.Context, client *godo.Client, cfg BotConfig) error {
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		return err
	}
	me, err := bot.GetMe()
	if err != nil {
		return err
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	// Since we work with single channel:
	sendMsg := func(v ...interface{}) {
		log.Print(v...)
		bot.Send(tgbotapi.NewMessage(cfg.TargetChat, fmt.Sprint(v...)))
	}

	upAction := func() error {
		return up(ctx, client, sendMsg, cfg.UpCfg)
	}

	downAction := func() error {
		return down(ctx, client, sendMsg, cfg.DownCfg)
	}

	updates := bot.GetUpdatesChan(u)
	isBusy := false
	workCh := make(chan error)

	for {
		select {
		case update := <-updates:
			if update.Message == nil {
				continue
			}

			if !update.Message.IsCommand() {
				continue
			}

			if !checkCmdValid(update.Message.CommandWithAt(), []string{"up", "down", "ping"}, me.UserName) {
				continue
			}

			chatID := update.Message.Chat.ID

			if chatID != cfg.TargetChat {
				bot.Send(tgbotapi.NewMessage(chatID, "I'm not permitted to work with you"))
				continue
			}

			if isBusy {
				bot.Send(tgbotapi.NewMessage(chatID, "I'm busy"))
				continue
			}

			cmd := update.Message.Command()

			if cmd == "up" {
				go runAction(upAction, workCh)
				isBusy = true
			}

			if cmd == "down" {
				go runAction(downAction, workCh)
				isBusy = true
			}

			if cmd == "ping" {
				sendMsg("pong")
			}
		case err = <-workCh:
			if err != nil {
				sendMsg(err.Error())
			} else {
				sendMsg("Done!")
			}

			isBusy = false
		}
	}
}
