package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/digitalocean/godo"
	_ "github.com/joho/godotenv/autoload"
)

var modeF = flag.String("m", "", "up | down")
var botF = flag.Bool("bot", false, "enable bot-mode")

type DownConfig struct {
	DropletName  string
	SnapshotName string
	DomainName   string
	HostName     string
}

type UpConfig struct {
	ProjectName  string
	DropletName  string
	DomainName   string
	HostName     string
	SnapshotName string
	Region       string
	Size         string
}

type BotConfig struct {
	Token      string
	TargetChat int64
	UpCfg      UpConfig
	DownCfg    DownConfig
}

func getVar(name string) string {
	v := os.Getenv(name)
	if v == "" {
		log.Fatal(name, " is missing")
	}

	return v
}

func main() {
	flag.Parse()

	dropletName := getVar("DROPLET_NAME")
	snapshotName := getVar("SNAPSHOT_NAME")
	domainName := getVar("DOMAIN_NAME")
	hostName := getVar("HOST_NAME")
	token := getVar("DIGITALOCEAN_TOKEN")
	projectName := getVar("PROJECT_NAME")
	region := getVar("REGION")
	size := getVar("SIZE")

	upCfg := UpConfig{
		ProjectName:  projectName,
		DropletName:  dropletName,
		DomainName:   domainName,
		HostName:     hostName,
		SnapshotName: snapshotName,
		Region:       region,
		Size:         size,
	}
	downCfg := DownConfig{
		DropletName:  dropletName,
		SnapshotName: snapshotName,
		DomainName:   domainName,
		HostName:     hostName,
	}

	client := godo.NewFromToken(token)
	ctx := context.TODO()

	if *botF {
		telegramToken := getVar("TELEGRAM_BOT_TOKEN")
		rawTelegramChatID := getVar("TELEGRAM_CHAT_ID")
		telegramChatID := int64(0)
		_, err := fmt.Sscanf(rawTelegramChatID, "%d", &telegramChatID)

		if err != nil {
			log.Fatal("failed to parse TELEGRAM_CHAT_ID: ", err.Error())
		}

		log.Print(telegramChatID)

		botCfg := BotConfig{
			Token:      telegramToken,
			TargetChat: telegramChatID,
			UpCfg:      upCfg,
			DownCfg:    downCfg,
		}

		bot(ctx, client, botCfg)
	} else if *modeF == "up" {
		err := up(ctx, client, log.Print, upCfg)
		if err != nil {
			log.Fatal(err.Error())
		}
	} else if *modeF == "down" {
		err := down(ctx, client, log.Print, downCfg)
		if err != nil {
			log.Fatal(err.Error())
		}
	} else {
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
}
