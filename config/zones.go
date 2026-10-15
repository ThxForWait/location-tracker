package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ZoneInfo описывает структуру одной локации из вашего JSON
type ZoneInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	PvpType string `json:"pvpType"`
	Tier    int    `json:"tier"`
}

// ZoneKind — категория локации с точки зрения карты.
type ZoneKind int

const (
	// ZoneInstance — всё, что не является узлом карты: данжи, острова,
	// хайдауты, арены, Мгла и неизвестные ID. На сервер не отправляется.
	ZoneInstance ZoneKind = iota
	// ZoneStatic — постоянная зона открытого мира (города, открытые зоны, проходы).
	ZoneStatic
	// ZoneRoad — зона Дорог Авалона (TNL-*).
	ZoneRoad
)

func (k ZoneKind) String() string {
	switch k {
	case ZoneStatic:
		return "static"
	case ZoneRoad:
		return "road"
	default:
		return "instance"
	}
}

// ZonesDatabase хранит карту всех зон в оперативной памяти
type ZonesDatabase struct {
	mu    sync.RWMutex
	zones map[string]ZoneInfo
}

var DB = &ZonesDatabase{
	zones: make(map[string]ZoneInfo),
}

// Load загружает данные из файла zones.json в память компьютера
func (db *ZonesDatabase) Load(filePath string) error {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать файл зон: %w", err)
	}
	return db.LoadBytes(fileBytes)
}

// LoadBytes загружает зоны из уже прочитанного JSON (например, встроенного в бинарник).
func (db *ZonesDatabase) LoadBytes(data []byte) error {
	zones := make(map[string]ZoneInfo)
	if err := json.Unmarshal(data, &zones); err != nil {
		return fmt.Errorf("ошибка парсинга JSON зон: %w", err)
	}

	db.mu.Lock()
	db.zones = zones
	db.mu.Unlock()

	fmt.Printf("[БАЗА ДАННЫХ] Успешно загружено локаций: %d\n", len(zones))
	return nil
}

// GetZoneName выполняет поиск имени по ID с учетом особенностей Albion Online
func (db *ZonesDatabase) GetZoneName(zoneID string) string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	id := strings.TrimSpace(zoneID)
	if id == "" {
		return "Неизвестная зона (пустой ID)"
	}

	// 1. Пробуем найти точное совпадение (например, "3004" или "ISLAND-PLAYER-0001a")
	if zone, exists := db.zones[id]; exists {
		return zone.Name
	}

	// 2. Логика отката (Fallback): если ID составной, например "1201-3" (подземелье в локации)
	// отрезаем дефис и ищем базовую локацию "1201"
	if strings.Contains(id, "-") {
		baseID := strings.Split(id, "-")[0]
		if zone, exists := db.zones[baseID]; exists {
			return fmt.Sprintf("%s (Данж/Инстанс)", zone.Name)
		}
	}

	// 3. Если это Мгла или динамический данж, которого нет в статичном JSON
	if strings.HasPrefix(id, "@MISTS") {
		return "Внутриигровая Мгла (Mists)"
	}

	return "Неизвестная локация [ID: " + id + "]"
}

// GetZoneKind определяет категорию зоны по полю type из zones.json.
// ID, которых нет в базе (хайдауты @HIDEOUT@..., Мгла @MISTS..., "1201-3" и т.п.),
// считаются инстансами.
func (db *ZonesDatabase) GetZoneKind(zoneID string) ZoneKind {
	db.mu.RLock()
	zone, exists := db.zones[strings.TrimSpace(zoneID)]
	db.mu.RUnlock()
	if !exists {
		return ZoneInstance
	}
	return classifyZoneType(zone.Type)
}

func classifyZoneType(zoneType string) ZoneKind {
	switch {
	case strings.HasPrefix(zoneType, "TUNNEL_"):
		return ZoneRoad
	case strings.HasPrefix(zoneType, "OPENPVP_"),
		strings.HasPrefix(zoneType, "PASSAGE_"),
		strings.HasPrefix(zoneType, "PLAYERCITY_") && !strings.Contains(zoneType, "HALL_OF_FAME"),
		zoneType == "STARTINGCITY",
		zoneType == "SAFEAREA",
		zoneType == "STARTAREA":
		return ZoneStatic
	}
	return ZoneInstance
}
