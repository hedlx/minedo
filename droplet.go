package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/godo"
)

var defaultListOpts = &godo.ListOptions{
	Page:    1,
	PerPage: 200,
}

func waitFor(ctx context.Context, client *godo.Client, dropletID, actionID int) error {
	for {
		action, _, err := client.DropletActions.Get(ctx, dropletID, actionID)

		if err != nil {
			return fmt.Errorf("failed to run action: %s", err.Error())
		}

		if action.Status == "completed" {
			return nil
		}

		time.Sleep(5 * time.Second)
	}
}

func waitForDroplet(ctx context.Context, client *godo.Client, dropletID int, status string) (*godo.Droplet, error) {
	for {
		droplet, _, err := client.Droplets.Get(ctx, dropletID)

		if err != nil {
			return nil, fmt.Errorf("failed to get droplet: %s", err.Error())
		}

		if droplet.Status == status {
			return droplet, nil
		}

		time.Sleep(5 * time.Second)
	}
}

func findDroplet(ctx context.Context, client *godo.Client, name string) (*godo.Droplet, error) {
	droplets, _, err := client.Droplets.List(ctx, defaultListOpts)

	if err != nil {
		return nil, fmt.Errorf("failed to get droplets: %s", err.Error())
	}

	var target *godo.Droplet
	for i, d := range droplets {
		if d.Name == name {
			target = &droplets[i]
			break
		}
	}

	return target, nil
}

func findSnapshot(ctx context.Context, client *godo.Client, name string) (*godo.Snapshot, error) {
	snapshots, _, err := client.Snapshots.List(ctx, defaultListOpts)

	if err != nil {
		return nil, fmt.Errorf("failed to get snapshots: %s", err.Error())
	}

	var target *godo.Snapshot
	for i, d := range snapshots {
		if d.Name == name {
			target = &snapshots[i]
			break
		}
	}

	return target, nil
}

func findProject(ctx context.Context, client *godo.Client, name string) (*godo.Project, error) {
	projects, _, err := client.Projects.List(ctx, defaultListOpts)

	if err != nil {
		return nil, fmt.Errorf("failed to get projects: %s", err.Error())
	}

	var target *godo.Project
	for i, d := range projects {
		if d.Name == name {
			target = &projects[i]
			break
		}
	}

	return target, nil
}

type LogFn = func(v ...interface{})

func up(ctx context.Context, client *godo.Client, logf LogFn, config UpConfig) error {
	snapshot, err := findSnapshot(ctx, client, config.SnapshotName)
	if err != nil {
		return err
	}
	if snapshot == nil {
		return fmt.Errorf("failed to find snapshot: %s", config.SnapshotName)
	}

	project, err := findProject(ctx, client, config.ProjectName)
	if err != nil {
		return err
	}
	if project == nil {
		return fmt.Errorf("failed to find project: %s", config.ProjectName)
	}

	droplet, err := findDroplet(ctx, client, config.DropletName)
	if err != nil {
		return err
	}
	if droplet != nil {
		return fmt.Errorf("droplet already exists: %s", config.DropletName)
	}

	snapshotID, err := strconv.Atoi(snapshot.ID)
	if err != nil {
		return fmt.Errorf("failed to convert snapshot ID: %s", snapshot.ID)
	}

	logf("Creating droplet from snapshot: ", snapshot.Name)
	droplet, _, err = client.Droplets.Create(ctx, &godo.DropletCreateRequest{
		Name:   config.DropletName,
		Region: config.Region,
		Size:   config.Size,
		Image: godo.DropletCreateImage{
			ID: snapshotID,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create droplet: %s", err.Error())
	}

	droplet, err = waitForDroplet(ctx, client, droplet.ID, "active")
	if err != nil {
		return err
	}

	logf("Droplet has been created: ", droplet.Name)

	_, _, err = client.Projects.AssignResources(ctx, project.ID, droplet)
	if err != nil {
		return fmt.Errorf("failed to assign droplet to project %s", err.Error())
	}

	logf("Droplet became a part of project: ", config.ProjectName)

	ip, err := droplet.PublicIPv4()

	if err != nil {
		return fmt.Errorf("failed to get public IPv4 of the droplet: %s", err.Error())
	}

	_, _, err = client.Domains.CreateRecord(ctx, config.DomainName, &godo.DomainRecordEditRequest{
		Type: "A",
		Name: strings.TrimSuffix(config.HostName, "."+config.DomainName),
		TTL:  3600,
		Data: ip,
	})

	if err != nil {
		return fmt.Errorf("failed to create 'A' record: %s", err.Error())
	}

	logf("'A' record has been created")

	_, err = client.Snapshots.Delete(ctx, snapshot.ID)

	if err != nil {
		return fmt.Errorf("failed to delete snapshot: %s", err.Error())
	}

	logf("Snapshot has been deleted: ", snapshot.Name)

	return nil
}

func down(ctx context.Context, client *godo.Client, logf LogFn, config DownConfig) error {
	droplet, err := findDroplet(ctx, client, config.DropletName)
	if err != nil {
		return nil
	}
	if droplet == nil {
		return fmt.Errorf("unable to find droplet: %s", config.DropletName)
	}

	snapshot, err := findSnapshot(ctx, client, config.SnapshotName)
	if err != nil {
		return err
	}
	if snapshot != nil {
		return fmt.Errorf("snapshot already exists: %s", config.SnapshotName)
	}

	logf("Found droplet: ", droplet.Name)

	if droplet.Status != "off" {
		logf("Shutting down droplet")
		shutdownAction, _, err := client.DropletActions.Shutdown(ctx, droplet.ID)
		if err != nil {
			return fmt.Errorf("failed to shutdown droplet: %s", err.Error())
		}

		waitFor(ctx, client, droplet.ID, shutdownAction.ID)
	}

	logf("Droplet is down")

	logf("Creating snapshot: ", config.SnapshotName)

	snapshotAction, _, err := client.DropletActions.Snapshot(ctx, droplet.ID, config.SnapshotName)

	if err != nil {
		return fmt.Errorf("failed to create snapshot: %s", err.Error())
	}

	waitFor(ctx, client, droplet.ID, snapshotAction.ID)

	// Double check
	snapshot, err = findSnapshot(ctx, client, config.SnapshotName)
	if err != nil {
		return err
	}
	if snapshot == nil {
		return fmt.Errorf("unable to find snapshot: %s", config.SnapshotName)
	}

	logf("Snapshot has been created")

	logf("Exterminating droplet")
	logf("E X T E R M I N A T E !")
	_, err = client.Droplets.Delete(ctx, droplet.ID)

	if err != nil {
		return fmt.Errorf("unable to exterminate droplet: %s", err.Error())
	}

	droplet, err = findDroplet(ctx, client, config.DropletName)
	if err != nil {
		return err
	}
	if droplet != nil {
		return fmt.Errorf("droplet still exists")
	}

	logf("Droplet has been exterminated")

	recID := -1
	recs, _, err := client.Domains.Records(ctx, config.DomainName, defaultListOpts)
	if err != nil {
		return fmt.Errorf("failed to get domain records")
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
			return fmt.Errorf("failed to delete record: %s", config.HostName)
		}

		logf("Record has been removed: ", config.HostName)
	}

	return nil
}
