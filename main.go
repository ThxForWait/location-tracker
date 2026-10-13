// main.go
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"location-tracker/capture"
	"location-tracker/config"
	"location-tracker/photon"
	"location-tracker/tracker"
	"location-tracker/api"
)

func main() {
	// Флаг для указания сетевого интерфейса
	iface := flag.String("iface", "auto", "Сетевой интерфейс или 'auto' для автовыбора")
	listFlag := flag.Bool("list", false, "Показать список интерфейсов и выйти")
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
	//файл с айдишниками локаций
	err := config.DB.Load("C:/PyProj/location-tracker/data/zones.json")
	if err != nil {
		log.Fatalf("Критическая ошибка старта: %v", err)
	}

	// Создаем канал для сырых пакетов
	packetChan := make(chan []byte, config.PacketChannelSize)

	// Инициализируем трекер (модуль 3)
	apiClient := api.NewClient(config.ApiUrl)
	tracker, err := tracker.NewTracker(apiClient)
	if err != nil {
		log.Fatalf("Ошибка инициализации трекера: %v", err)
	}
	defer tracker.Close()

	// Инициализируем парсер (модуль 2)
	// Передаем ему обработчик событий из трекера
	//photonParser := photon.NewParser(nil, tracker.HandleResponse)
	photonParser := photon.NewPhotonParser(
		func(ev *photon.EventData) { // промежуточный колбек
			photon.PostProcessEvent(ev)// полуобработаные данные
			photon.FinalizeEvent(ev)// тут функция которая обрабанывает данные окончательно
			tracker.OnEvent(ev) // уже настоящий кол бек получает полность обработанные данны
		},
		nil,
		func(resp *photon.OperationResponse) { // промежуточный колбек
			photon.PostProcessResponse(resp)// полуобработаные данные
			photon.FinalizeResponse(resp)// тут функция которая обрабанывает данные окончательно
			tracker.OnResponse(resp) // уже настоящий кол бек получает полность обработанные данны
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
			//log.Printf("новый пакет")
		}
	}()

	// Ожидаем сигнал завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Завершение работы...")
}
