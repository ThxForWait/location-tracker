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
)

func main() {
	// Флаг для указания сетевого интерфейса
	iface := flag.String("iface", "", "Сетевой интерфейс для захвата (например, \\Device\\NPF_{...})")
	flag.Parse()

	if *iface == "" {
		log.Fatal("Укажите сетевой интерфейс через флаг -iface")
	}
	//файл с айдишниками локаций
	err := config.DB.Load("C:/PyProj/location-tracker/data/zones.json")
	if err != nil {
		log.Fatalf("Критическая ошибка старта: %v", err)
	}

	// Создаем канал для сырых пакетов
	packetChan := make(chan []byte, config.PacketChannelSize)

	// Инициализируем трекер (модуль 3)
	tracker, err := tracker.NewTracker()
	if err != nil {
		log.Fatalf("Ошибка инициализации трекера: %v", err)
	}
	defer tracker.Close()

	// Инициализируем парсер (модуль 2)
	// Передаем ему обработчик событий из трекера
	photonParser := photon.NewParser(nil, tracker.HandleResponse)

	// Запускаем захват пакетов (модуль 1) в отдельной горутине
	go func() {
		if err := capture.StartCapture(*iface, packetChan); err != nil {
			log.Fatalf("Ошибка захвата пакетов: %v", err)
		}
	}()

	log.Println("Трекер локаций запущен. Ожидание переходов...")
	log.Println("Нажмите Ctrl+C для выхода.")

	// Основной цикл: читаем сырые пакеты и отправляем их в парсер
	go func() {
		for payload := range packetChan {
			photonParser.ParsePayload(payload)
		}
	}()

	// Ожидаем сигнал завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Завершение работы...")
}