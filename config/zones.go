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

// ZonesDatabase хранит карту всех зон в оперативной памяти
type ZonesDatabase struct {
	mu    sync.RWMutex
	zones map[string]ZoneInfo
}

var DB = &ZonesDatabase{
	zones: make(map[string]ZoneInfo),
}

// LoadЗагружает данные из zones.json в память компьютера
func (db *ZonesDatabase) Load(filePath string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать файл зон: %w", err)
	}

	// Десериализуем JSON прямо в нашу карту Go
	if err := json.Unmarshal(fileBytes, &db.zones); err != nil {
		return fmt.Errorf("ошибка парсинга JSON зон: %w", err)
	}

	fmt.Printf("[БАЗА ДАННЫХ] Успешно загружено локаций: %d\n", len(db.zones))
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
