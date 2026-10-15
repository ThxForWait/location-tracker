// main.go
package main

import (
	"context"
	_ "embed"
	"flag"
	"log"
	"os/signal"
	"syscall"
	"time"

	"location-tracker/api"
	"location-tracker/capture"
	"location-tracker/config"
	"location-tracker/photon"
	"location-tracker/tracker"
)

// База зон встроена в бинарник, чтобы exe работал без папки data рядом.
//
//go:embed data/zones.json
var embeddedZones []byte

func main() {
	// Флаг для указания сетевого интерфейса
	iface := flag.String("iface", "auto", "Сетевой интерфейс или 'auto', чтобы слушать все и выбирать активный на лету")
	listFlag := flag.Bool("list", false, "Показать список интерфейсов и выйти")
	debug := flag.Bool("debug", false, "Отправлять данные на локальный сервер ("+config.DebugApiUrl+")")
	apiURL := flag.String("api", "", "URL API (по умолчанию боевой сервер, с -debug — локальный)")
	zonesPath := flag.String("zones", "", "Путь к zones.json (по умолчанию встроенный)")
	flag.Parse()
	if *listFlag {
		if err := capture.PrintList(); err != nil {
			log.Fatalf("Ошибка: %v", err)
		}
		return
	}
	// "" — слушать все интерфейсы
	onlyIface := *iface
	if onlyIface == "auto" {
		onlyIface = ""
	}

	// База зон: встроенная или из файла
	var err error
	if *zonesPath != "" {
		err = config.DB.Load(*zonesPath)
	} else {
		err = config.DB.LoadBytes(embeddedZones)
	}
	if err != nil {
		log.Fatalf("Критическая ошибка старта: %v", err)
	}

	// Создаем канал для сырых пакетов
	packetChan := make(chan capture.Packet, config.PacketChannelSize)

	// Инициализируем трекер (модуль 3)
	url := *apiURL
	if url == "" {
		url = config.ApiUrl
		if *debug {
			url = config.DebugApiUrl
		}
	}
	apiClient := api.NewClient(url)
	sender := tracker.NewSender(apiClient)
	tracker, err := tracker.NewTracker(sender)
	if err != nil {
		log.Fatalf("Ошибка инициализации трекера: %v", err)
	}
	defer tracker.Close()

	// Инициализируем парсер (модуль 2)
	// События пока не используются, поэтому их колбек не передаём.
	newParser := func() *photon.PhotonParser {
		return photon.NewPhotonParser(
			nil,
			nil,
			func(resp *photon.OperationResponse) {
				photon.FinalizeResponse(resp) // внутри уже вызывается PostProcessResponse
				tracker.OnResponse(resp)
			},
		)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Запускаем захват пакетов (модуль 1) со всех интерфейсов в отдельной горутине
	captureDone := make(chan struct{})
	go func() {
		defer close(captureDone)
		capture.Start(ctx, onlyIface, packetChan)
	}()

	log.Println("Трекер локаций запущен. Ожидание переходов...")
	log.Println("Нажмите Ctrl+C для выхода.")

	// Основной цикл: берём пакеты только с активного интерфейса и отправляем их в парсер
	go func() {
		selector := capture.NewSelector(config.InterfaceSwitchAfter)
		photonParser := newParser()
		for pkt := range packetChan {
			accept, switched := selector.Accept(pkt.Iface, time.Now())
			if !accept {
				continue
			}
			if switched {
				// Недособранные фрагменты со старого интерфейса больше не придут
				photonParser = newParser()
			}
			photonParser.ReceivePacket(pkt.Payload)
		}
	}()

	// Ожидаем сигнал завершения
	<-ctx.Done()

	log.Println("Завершение работы...")
	<-captureDone
	sender.Shutdown(config.SendShutdownGrace)
}
