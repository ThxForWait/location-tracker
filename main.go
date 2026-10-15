// main.go
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"location-tracker/api"
	"location-tracker/auth"
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
	loginFlag := flag.Bool("login", false, "Заново подключить трекер к аккаунту через браузер")
	tokenFlag := flag.String("token", "", "Токен устройства (вместо входа через браузер, не сохраняется)")
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("Версия трекера: %s", config.Version)
	apiClient := api.NewClient(url, config.Version)
	if err := connectAccount(ctx, apiClient, *loginFlag, *tokenFlag); err != nil {
		log.Fatalf("Не удалось подключиться к аккаунту: %v", err)
	}
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

// connectAccount ставит клиенту токен: из -token, из сохранённых или через вход в браузере.
func connectAccount(ctx context.Context, client *api.Client, forceLogin bool, token string) error {
	path, err := auth.DefaultPath()
	if err != nil {
		return err
	}
	deviceName, _ := os.Hostname()
	err = auth.EnsureToken(ctx, client, auth.Options{
		Path:        path,
		ForceLogin:  forceLogin,
		Token:       token,
		DeviceName:  deviceName,
		OpenBrowser: auth.OpenBrowser,
	})
	switch {
	case api.IsOutdated(err):
		return fmt.Errorf("трекер устарел, скачайте новую версию: %w", err)
	case api.IsAuthError(err):
		return fmt.Errorf("токен не принят сервером (запустите с -login): %w", err)
	}
	return err
}
