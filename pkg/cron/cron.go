package cron

import (
	"log"

	"github.com/robfig/cron/v3"
)

type CronManager struct {
	cron *cron.Cron
}

func NewCronManager() *CronManager {
	return &CronManager{
		cron: cron.New(cron.WithSeconds()),
	}
}

func (cm *CronManager) Start() {
	cm.cron.Start()
	log.Println("Cron manager started")
}

func (cm *CronManager) Stop() {
	cm.cron.Stop()
	log.Println("Cron manager stopped")
}

func (cm *CronManager) AddJob(spec string, cmd func()) (cron.EntryID, error) {
	return cm.cron.AddFunc(spec, cmd)
}
