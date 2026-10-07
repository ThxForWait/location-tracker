// list_devices.go
package main

import (
	"fmt"
	"log"

	"github.com/google/gopacket/pcap"
)

func mainsdsa() {
	// ВАЖНО: запускать от имени администратора!
	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Доступные сетевые интерфейсы:")
	fmt.Println("================================")
	for i, dev := range devices {
		fmt.Printf("\n[%d] Name: %s\n", i, dev.Name)
		fmt.Printf("    Description: %s\n", dev.Description)
		if len(dev.Addresses) > 0 {
			fmt.Printf("    IP-адреса:\n")
			for _, addr := range dev.Addresses {
				fmt.Printf("      - %s\n", addr.IP)
			}
		}
	}
}