// capture/auto.go
package capture

import (
	"fmt"
	"log"
	"net"
	"sort"
	"strings"

	"github.com/google/gopacket/pcap"
)

// InterfaceKind — категория сетевого интерфейса, влияет на приоритет выбора.
type InterfaceKind int

const (
	KindUnknown InterfaceKind = iota
	KindLoopback
	KindVirtual
	KindWireless
	KindEthernet
)

func (k InterfaceKind) String() string {
	switch k {
	case KindLoopback:
		return "loopback"
	case KindVirtual:
		return "virtual"
	case KindWireless:
		return "wireless"
	case KindEthernet:
		return "ethernet"
	default:
		return "unknown"
	}
}

// InterfaceInfo — сведения об интерфейсе для отображения и выбора.
type InterfaceInfo struct {
	Name        string
	Description string
	IPs         []net.IP
	Kind        InterfaceKind
}

// HasRoutableIP сообщает, есть ли у интерфейса хоть один «настоящий» IPv4/IPv6-адрес.
func (i InterfaceInfo) HasRoutableIP() bool {
	for _, ip := range i.IPs {
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			continue
		}
		return true
	}
	return false
}

// List возвращает список всех доступных интерфейсов с классификацией.
func List() ([]InterfaceInfo, error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return nil, fmt.Errorf("не удалось получить список интерфейсов: %w", err)
	}

	out := make([]InterfaceInfo, 0, len(devices))
	for _, dev := range devices {
		info := InterfaceInfo{
			Name:        dev.Name,
			Description: dev.Description,
		}
		for _, addr := range dev.Addresses {
			info.IPs = append(info.IPs, addr.IP)
		}
		info.Kind = classify(info)
		out = append(out, info)
	}

	// Сортируем: сначала «хорошие» интерфейсы, потом всё остальное.
	sort.SliceStable(out, func(a, b int) bool {
		return rank(out[a]) < rank(out[b])
	})

	return out, nil
}

// Auto выбирает наиболее подходящий интерфейс для захвата.
// Возвращает ошибку, если подходящий интерфейс не найден.
func Auto() (string, error) {
	ifaces, err := List()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if !iface.HasRoutableIP() {
			continue
		}
		if iface.Kind == KindEthernet || iface.Kind == KindWireless {
			log.Printf("[capture] Автовыбор: %s (%s, IP=%v)",
				iface.Description, iface.Kind, iface.IPs)
			return iface.Name, nil
		}
	}

	// Fallback: любой интерфейс с хоть каким-то IP.
	for _, iface := range ifaces {
		if iface.HasRoutableIP() {
			log.Printf("[capture] Автовыбор (fallback): %s (%s, IP=%v)",
				iface.Description, iface.Kind, iface.IPs)
			return iface.Name, nil
		}
	}

	return "", fmt.Errorf("не найдено ни одного подходящего сетевого интерфейса")
}

// PrintList выводит список интерфейсов в stdout (для флага -list).
func PrintList() error {
	ifaces, err := List()
	if err != nil {
		return err
	}

	fmt.Println("Доступные сетевые интерфейсы:")
	fmt.Println("================================")
	for i, iface := range ifaces {
		marker := "  "
		if iface.HasRoutableIP() && (iface.Kind == KindEthernet || iface.Kind == KindWireless) {
			marker = "* "
		}
		fmt.Printf("%s[%d] Name: %s\n", marker, i, iface.Name)
		fmt.Printf("      Description: %s\n", iface.Description)
		fmt.Printf("      Kind: %s\n", iface.Kind)
		for _, ip := range iface.IPs {
			fmt.Printf("      IP: %s\n", ip)
		}
		fmt.Println()
	}
	fmt.Println("Интерфейсы, помеченные * — рекомендуемые для захвата.")
	return nil
}

// classify определяет категорию интерфейса по описанию и адресам.
func classify(info InterfaceInfo) InterfaceKind {
	desc := strings.ToLower(info.Description)

	switch {
	case strings.Contains(desc, "loopback") || strings.Contains(info.Name, "Loopback"):
		return KindLoopback
	case strings.Contains(desc, "wan miniport"),
		strings.Contains(desc, "virtualbox"),
		strings.Contains(desc, "vmware"),
		strings.Contains(desc, "hyper-v"),
		strings.Contains(desc, "vpn"),
		strings.Contains(desc, "tunnel"),
		strings.Contains(desc, "tap-"),
		strings.Contains(desc, "bluetooth"):
		return KindVirtual
	case strings.Contains(desc, "wi-fi"),
		strings.Contains(desc, "wifi"),
		strings.Contains(desc, "wireless"),
		strings.Contains(desc, "802.11"):
		return KindWireless
	case strings.Contains(desc, "ethernet"),
		strings.Contains(desc, "gbe"),
		strings.Contains(desc, "realtek"),
		strings.Contains(desc, "intel"),
		strings.Contains(desc, "broadcom"):
		return KindEthernet
	}
	return KindUnknown
}

// rank нужен для сортировки: чем меньше число, тем выше интерфейс в списке.
func rank(info InterfaceInfo) int {
	if !info.HasRoutableIP() {
		return 100
	}
	switch info.Kind {
	case KindEthernet:
		return 0
	case KindWireless:
		return 1
	case KindUnknown:
		return 2
	case KindVirtual:
		return 10
	case KindLoopback:
		return 20
	}
	return 50
}