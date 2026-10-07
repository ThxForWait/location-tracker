// capture/capture.go
package capture

import (
	"log"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	"location-tracker/config"
)

// StartCapture запускает захват пакетов на указанном интерфейсе
// и отправляет сырые UDP payload'ы в канал packetChan.
func StartCapture(iface string, packetChan chan<- []byte) error {
	// Открываем устройство для захвата
	handle, err := pcap.OpenLive(iface, 65536, false, pcap.BlockForever)
	if err != nil {
		return err
	}

	// Устанавливаем BPF-фильтр
	if err := handle.SetBPFFilter(config.BPFFilter); err != nil {
		handle.Close()
		return err
	}

	log.Printf("Захват запущен на интерфейсе %s с фильтром: %s", iface, config.BPFFilter)

	// Создаем источник пакетов
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetSource.DecodeOptions.Lazy = true
	packetSource.DecodeOptions.NoCopy = true

	// Читаем пакеты в цикле
	go func() {
		defer handle.Close()
		for packet := range packetSource.Packets() {
			// Извлекаем UDP-слой
			udpLayer := packet.Layer(layers.LayerTypeUDP)
			if udpLayer == nil {
				continue
			}

			udp, _ := udpLayer.(*layers.UDP)
			payload := udp.Payload

			if len(payload) > 0 {
				// Копируем срез, так как gopacket может переиспользовать буфер
				payloadCopy := make([]byte, len(payload))
				copy(payloadCopy, payload)
				packetChan <- payloadCopy
			}
		}
	}()

	return nil
}