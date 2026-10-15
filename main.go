// main.go
package main

import (
	_ "embed"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	iface := flag.String("iface", "auto", "Сетевой интерфейс или 'auto' для автовыбора")
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
	ifaceName := *iface
	if ifaceName == "auto" || ifaceName == "" {
		chosen, err := capture.Auto()
		if err != nil {
			log.Fatalf("Автовыбор интерфейса не удался: %v. Запустите с -list, чтобы увидеть список.", err)
		}
		ifaceName = chosen
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
	packetChan := make(chan []byte, config.PacketChannelSize)

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
	photonParser := photon.NewPhotonParser(
		nil,
		nil,
		func(resp *photon.OperationResponse) {
			photon.FinalizeResponse(resp) // внутри уже вызывается PostProcessResponse
			tracker.OnResponse(resp)
		},
	)

	// Запускаем захват пакетов (модуль 1) в отдельной горутине
	go func() {
		if err := capture.StartCapture(ifaceName, packetChan); err != nil {
			log.Fatalf("Ошибка захвата пакетов: %v", err)
		}
	}()

	log.Println("Трекер локаций запущен. Ожидание переходов...")
	log.Println("Нажмите Ctrl+C для выхода.")

	// Основной цикл: читаем сырые пакеты и отправляем их в парсер
	go func() {
		for payload := range packetChan {
			photonParser.ReceivePacket(payload)
		}
	}()

	// Ожидаем сигнал завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Завершение работы...")
	sender.Shutdown(config.SendShutdownGrace)
}
