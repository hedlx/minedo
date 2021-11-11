package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	_ "github.com/joho/godotenv/autoload"
)

var modeF = flag.String("m", "", "up | down")

var opt = &godo.ListOptions{
	Page:    1,
	PerPage: 200,
}

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

func waitFor(ctx context.Context, client *godo.Client, dropletID, actionID int) {
	for {
		action, _, err := client.DropletActions.Get(ctx, dropletID, actionID)

		if err != nil {
			log.Fatal("Failed to run action: ", err.Error())
		}

		if action.Status == "completed" {
			return
		}

		time.Sleep(5 * time.Second)
	}
}

func waitForDroplet(ctx context.Context, client *godo.Client, dropletID int, status string) *godo.Droplet {
	for {
		droplet, _, err := client.Droplets.Get(ctx, dropletID)

		if err != nil {
			log.Fatal("Failed to get droplet: ", err.Error())
		}

		if droplet.Status == status {
			return droplet
		}

		time.Sleep(5 * time.Second)
	}
}

func findDroplet(ctx context.Context, client *godo.Client, name string) *godo.Droplet {
	droplets, _, err := client.Droplets.List(ctx, opt)

	if err != nil {
		log.Fatal("Failed to get droplets: ", err.Error())
	}

	var target *godo.Droplet
	for i, d := range droplets {
		if d.Name == name {
			target = &droplets[i]
			break
		}
	}

	return target
}

func findSnapshot(ctx context.Context, client *godo.Client, name string) *godo.Snapshot {
	snapshots, _, err := client.Snapshots.List(ctx, opt)

	if err != nil {
		log.Fatal("Failed to get snapshots: ", err.Error())
	}

	var target *godo.Snapshot
	for i, d := range snapshots {
		if d.Name == name {
			target = &snapshots[i]
			break
		}
	}

	return target
}

func findProject(ctx context.Context, client *godo.Client, name string) *godo.Project {
	projects, _, err := client.Projects.List(ctx, opt)

	if err != nil {
		log.Fatal("Failed to get projects: ", err.Error())
	}

	var target *godo.Project
	for i, d := range projects {
		if d.Name == name {
			target = &projects[i]
			break
		}
	}

	return target
}

func up(ctx context.Context, client *godo.Client, config UpConfig) {
	snapshot := findSnapshot(ctx, client, config.SnapshotName)
	if snapshot == nil {
		log.Fatal("Failed to find snapshot: ", config.SnapshotName)
	}

	project := findProject(ctx, client, config.ProjectName)
	if project == nil {
		log.Fatal("Failed to find project: ", config.ProjectName)
	}

	droplet := findDroplet(ctx, client, config.DropletName)
	if droplet != nil {
		log.Fatal("Droplet already exists: ", config.DropletName)
	}

	snapshotID, err := strconv.Atoi(snapshot.ID)
	if err != nil {
		log.Fatal("Failed to convert snapshot ID: ", snapshot.ID)
	}

	log.Print("Creating droplet from snapshot: ", snapshot.Name)
	droplet, _, err = client.Droplets.Create(ctx, &godo.DropletCreateRequest{
		Name:   config.DropletName,
		Region: config.Region,
		Size:   config.Size,
		Image: godo.DropletCreateImage{
			ID: snapshotID,
		},
	})

	if err != nil {
		log.Fatal("Failed to create droplet: ", err.Error())
	}

	droplet = waitForDroplet(ctx, client, droplet.ID, "active")

	log.Print("Droplet has been created: ", droplet.Name)

	_, _, err = client.Projects.AssignResources(ctx, project.ID, droplet)
	if err != nil {
		log.Fatal("Failed to assign droplet to project", err.Error())
	}

	log.Print("Droplet became a part of project: ", config.ProjectName)

	ip, err := droplet.PublicIPv4()

	if err != nil {
		log.Fatal("Failed to get public IPv4 of the droplet: ", err.Error())
	}

	_, _, err = client.Domains.CreateRecord(ctx, config.DomainName, &godo.DomainRecordEditRequest{
		Type: "A",
		Name: strings.TrimSuffix(config.HostName, "."+config.DomainName),
		TTL:  3600,
		Data: ip,
	})

	if err != nil {
		log.Fatal("Failed to create 'A' record: ", err.Error())
	}

	log.Print("'A' record has been created")

	_, err = client.Snapshots.Delete(ctx, snapshot.ID)

	if err != nil {
		log.Fatal("Failed to delete snapshot: ", err.Error())
	}

	log.Print("Snapshot has been deleted: ", snapshot.Name)
}

func down(ctx context.Context, client *godo.Client, config DownConfig) {
	droplet := findDroplet(ctx, client, config.DropletName)
	if droplet == nil {
		log.Fatal("Unable to find droplet: ", config.DropletName)
	}

	snapshot := findSnapshot(ctx, client, config.SnapshotName)
	if snapshot != nil {
		log.Fatal("Snapshot already exists: ", config.SnapshotName)
	}

	log.Print("Found droplet: ", droplet.Name)

	if droplet.Status != "off" {
		log.Print("Shutting down droplet")
		shutdownAction, _, err := client.DropletActions.Shutdown(ctx, droplet.ID)
		if err != nil {
			log.Fatal("Failed to shutdown droplet: ", err.Error())
		}

		waitFor(ctx, client, droplet.ID, shutdownAction.ID)
	}

	log.Print("Droplet is down")

	log.Print("Creating snapshot: ", config.SnapshotName)

	snapshotAction, _, err := client.DropletActions.Snapshot(ctx, droplet.ID, config.SnapshotName)

	if err != nil {
		log.Fatal("Failed to create snapshot: ", err.Error())
	}

	waitFor(ctx, client, droplet.ID, snapshotAction.ID)

	// Double check
	snapshot = findSnapshot(ctx, client, config.SnapshotName)
	if snapshot == nil {
		log.Fatal("Unable to find snapshot: ", config.SnapshotName)
	}

	log.Print("Snapshot has been created")

	log.Print("Exterminating droplet")
	log.Print("E X T E R M I N A T E !")
	_, err = client.Droplets.Delete(ctx, droplet.ID)

	if err != nil {
		log.Fatal("Unable to exterminate droplet: ", err.Error())
	}

	droplet = findDroplet(ctx, client, config.DropletName)
	if droplet != nil {
		log.Fatal("Droplet still exists!")
	}

	log.Print("Droplet has been exterminated")

	recID := -1
	recs, _, err := client.Domains.Records(ctx, config.DomainName, opt)
	if err != nil {
		log.Fatal("Failed to get domain records")
	}

	for _, r := range recs {
		if r.Name+"."+config.DomainName == config.HostName {
			recID = r.ID
			break
		}
	}

	if recID > -1 {
		_, err := client.Domains.DeleteRecord(ctx, config.DomainName, recID)

		if err != nil {
			log.Fatal("Failed to delete record: ", config.HostName)
		}

		log.Print("Record has been removed: ", config.HostName)
	}
}

func main() {
	flag.Parse()

	dropletName := os.Getenv("COPROSERVER_NAME")
	if dropletName == "" {
		log.Fatal("COPROSERVER_NAME is missing")
	}

	snapshotName := os.Getenv("SNAPSHOT_NAME")
	if snapshotName == "" {
		log.Fatal("SNAPSHOT_NAME is missing")
	}

	domainName := os.Getenv("DOMAIN_NAME")
	if domainName == "" {
		log.Fatal("DOMAIN_NAME is missing")
	}

	hostName := os.Getenv("HOST_NAME")
	if hostName == "" {
		log.Fatal("HOST_NAME is missing")
	}

	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		log.Fatal("DIGITALOCEAN_TOKEN is missing")
	}

	client := godo.NewFromToken(token)
	ctx := context.TODO()

	if *modeF == "up" {
		projectName := os.Getenv("PROJECT_NAME")
		if projectName == "" {
			log.Fatal("PROJECT_NAME is missing")
		}

		region := os.Getenv("REGION")
		if region == "" {
			log.Fatal("REGION is missing")
		}

		size := os.Getenv("SIZE")
		if size == "" {
			log.Fatal("SIZE is missing")
		}
		up(ctx, client, UpConfig{
			ProjectName:  projectName,
			DropletName:  dropletName,
			DomainName:   domainName,
			HostName:     hostName,
			SnapshotName: snapshotName,
			Region:       region,
			Size:         size,
		})
	} else if *modeF == "down" {
		down(ctx, client, DownConfig{
			DropletName:  dropletName,
			SnapshotName: snapshotName,
			DomainName:   domainName,
			HostName:     hostName,
		})
	} else {
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
}