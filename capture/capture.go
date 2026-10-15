// capture/capture.go
package capture

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"

	"location-tracker/config"
)

const (
	snapLen = 65536
	// Таймаут чтения: без него Close на Windows может зависнуть в BlockForever.
	readTimeout = 500 * time.Millisecond
	// Буфер ядра на один интерфейс. С фильтром по игровому порту этого с запасом хватает.
	bufferSize = 1 << 20
)

// Packet — UDP payload вместе с интерфейсом, с которого он пришёл.
type Packet struct {
	Iface   string
	Payload []byte
}

// multiCapture держит открытыми все интерфейсы и периодически ищет новые
// (VPN-адаптеры часто появляются только в момент подключения).
type multiCapture struct {
	out  chan<- Packet
	only string // если задан, захват только с этого интерфейса

	mu      sync.Mutex
	running map[string]chan struct{} // интерфейс → канал остановки его горутины
	failed  map[string]bool          // интерфейсы, об ошибке открытия которых уже написали
	wg      sync.WaitGroup
}

// Start захватывает игровой трафик со всех интерфейсов (или только с only, если он задан)
// и отправляет UDP payload'ы в out. Блокируется до отмены ctx, затем закрывает все интерфейсы.
func Start(ctx context.Context, only string, out chan<- Packet) {
	m := &multiCapture{
		out:     out,
		only:    only,
		running: make(map[string]chan struct{}),
		failed:  make(map[string]bool),
	}
	log.Printf("[capture] Фильтр захвата: %s", config.BPFFilter)

	m.rescan()
	ticker := time.NewTicker(config.InterfaceRescanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return
		case <-ticker.C:
			m.rescan()
		}
	}
}

// rescan открывает новые интерфейсы и останавливает исчезнувшие.
func (m *multiCapture) rescan() {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		log.Printf("[capture] Не удалось получить список интерфейсов: %v", err)
		return
	}

	present := make(map[string]bool, len(devices))
	for _, dev := range devices {
		if m.only != "" && dev.Name != m.only {
			continue
		}
		present[dev.Name] = true
		rememberDescription(dev.Name, dev.Description)

		m.mu.Lock()
		_, isRunning := m.running[dev.Name]
		m.mu.Unlock()
		if isRunning {
			continue
		}
		m.open(dev.Name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for name, stop := range m.running {
		if !present[name] {
			log.Printf("[capture] Интерфейс пропал: %s", Describe(name))
			close(stop)
			delete(m.running, name)
		}
	}
	if m.only != "" && len(m.running) == 0 && !m.failed[m.only] {
		log.Printf("[capture] Интерфейс %s не найден", m.only)
		m.failed[m.only] = true
	}
}

func (m *multiCapture) open(name string) {
	handle, err := openHandle(name)

	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		// Пишем об ошибке один раз: WAN Miniport и подобные не откроются никогда.
		if !m.failed[name] {
			log.Printf("[capture] Пропущен интерфейс %s: %v", Describe(name), err)
			m.failed[name] = true
		}
		return
	}
	delete(m.failed, name)

	stop := make(chan struct{})
	m.running[name] = stop
	m.wg.Add(1)
	go m.read(name, handle, stop)
	log.Printf("[capture] Слушаю интерфейс: %s", Describe(name))
}

func openHandle(name string) (*pcap.Handle, error) {
	inactive, err := pcap.NewInactiveHandle(name)
	if err != nil {
		return nil, err
	}
	defer inactive.CleanUp()

	if err := inactive.SetSnapLen(snapLen); err != nil {
		return nil, err
	}
	if err := inactive.SetPromisc(false); err != nil {
		return nil, err
	}
	if err := inactive.SetTimeout(readTimeout); err != nil {
		return nil, err
	}
	if err := inactive.SetBufferSize(bufferSize); err != nil {
		return nil, err
	}
	if err := inactive.SetImmediateMode(true); err != nil {
		return nil, err
	}

	handle, err := inactive.Activate()
	if err != nil {
		return nil, err
	}
	if err := handle.SetBPFFilter(config.BPFFilter); err != nil {
		handle.Close()
		return nil, err
	}
	return handle, nil
}

// read читает пакеты с одного интерфейса, пока его не остановят или он не пропадёт.
// Handle закрывается здесь же, чтобы Close никогда не шёл параллельно с чтением.
func (m *multiCapture) read(name string, handle *pcap.Handle, stop <-chan struct{}) {
	defer m.wg.Done()
	defer handle.Close()

	linkType := handle.LinkType()
	for {
		select {
		case <-stop:
			return
		default:
		}

		data, _, err := handle.ReadPacketData()
		if errors.Is(err, pcap.NextErrorTimeoutExpired) {
			continue
		}
		if err != nil {
			log.Printf("[capture] Интерфейс %s перестал отвечать: %v", Describe(name), err)
			m.mu.Lock()
			if m.running[name] == stop {
				delete(m.running, name)
			}
			m.mu.Unlock()
			return
		}

		payload := udpPayload(data, linkType)
		if len(payload) == 0 {
			continue
		}
		// Копируем срез, так как pcap может переиспользовать буфер
		pkt := Packet{Iface: name, Payload: append([]byte(nil), payload...)}
		select {
		case m.out <- pkt:
		case <-stop:
			return
		}
	}
}

func (m *multiCapture) stopAll() {
	m.mu.Lock()
	for name, stop := range m.running {
		close(stop)
		delete(m.running, name)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// udpPayload достаёт UDP payload с учётом типа канального уровня.
// VPN-адаптеры (Wintun, TAP) часто отдают «сырой IP» с DLT_RAW = 12,
// который gopacket не знает, поэтому при неудаче пробуем разобрать данные как голый IP.
func udpPayload(data []byte, linkType layers.LinkType) []byte {
	opts := gopacket.DecodeOptions{Lazy: true, NoCopy: true}
	if p := udpFrom(gopacket.NewPacket(data, linkType, opts)); p != nil {
		return p
	}
	if len(data) == 0 {
		return nil
	}
	switch data[0] >> 4 {
	case 4:
		return udpFrom(gopacket.NewPacket(data, layers.LayerTypeIPv4, opts))
	case 6:
		return udpFrom(gopacket.NewPacket(data, layers.LayerTypeIPv6, opts))
	}
	return nil
}

func udpFrom(packet gopacket.Packet) []byte {
	if udp, ok := packet.Layer(layers.LayerTypeUDP).(*layers.UDP); ok {
		return udp.Payload
	}
	return nil
}

var descriptions sync.Map // имя интерфейса → описание

func rememberDescription(name, desc string) {
	if desc != "" {
		descriptions.Store(name, desc)
	}
}

// Describe возвращает человекочитаемое описание интерфейса для логов.
func Describe(name string) string {
	if d, ok := descriptions.Load(name); ok {
		return d.(string)
	}
	return name
}
