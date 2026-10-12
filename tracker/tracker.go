// tracker/tracker.go
package tracker

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"location-tracker/config"
	"location-tracker/photon"
	"location-tracker/api"
)

// Tracker обрабатывает события от парсера и записывает переходы в файл.
type Tracker struct {
	logFile *os.File
	mu      sync.Mutex
	currentZoneID string
	NextZoneID string
	apiClient api.Client
}

// NewTracker создает новый трекер и открывает файл для записи.
func NewTracker(apiClient *api.Client) (*Tracker, error) {
	file, err := os.OpenFile(confifunc NewTracker() (*Tracker, error) {
		file, err := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("не удалось открыть файл лога: %w", err)
		}

		log.Printf("Логирование переходов будет вестись в файл: %s", config.LogFilePath)

		return &Tracker{
			logFile: file,
			currentZoneID: "unknown",
			NextZoneID: "unknown",
			apiClient: api.Client
		}, nil
	}g.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл лога: %w", err)
	}

	log.Printf("Логирование переходов будет вестись в файл: %s", config.LogFilePath)

	return &Tracker{
		logFile: file,
		currentZoneID: "unknown",
		NextZoneID: "unknown",
	}, nil
}

// HandleEvent обрабатывает одно событие от Photon-парсера.
// code — код события, parameters — срез параметров.
func (t *Tracker) OnEvent(ev *photon.EventData) {
	//log.Printf("EVENTDATA: %v+", ev)
}

func (t *Tracker) OnResponse(resp *photon.OperationResponse) {
	albionOp := photon.GetInt(resp.Parameters, 253)
	if albionOp == 0 {
		albionOp = int64(resp.OperationCode)
	}
	clusterId := ""
	switch albionOp{

		case 41:
			clusterId = photon.GetStr(resp.Parameters, 0)
			if clusterId == ""  {log.Printf("Ошибка в считывании ID локации, OpCode 41"); return}
			t.NextZoneID = clusterId
		case 2:
			clusterId = photon.GetStr(resp.Parameters, 8)
			if clusterId == ""  {log.Printf("Ошибка в считывании ID локации, OpCode 2"); return}
			if clusterId != t.NextZoneID {
				log.Printf("Ошибка, ID в OpCode 41 и 2 не совпало")
				t.currentZoneID = clusterId
				t.NextZoneID = "unknown"
			}
			t.saveLocation(clusterId)
		default :
			//log.Printf("Необработанный пакет OnResponse code: %d , data %v+", albionOp, resp.Parameters)
	}


}

func (t *Tracker) handleZoneChange(parameters []interface{}) {
	if len(parameters) == 0 {
		return
	}

	var newZoneID string = "unknown"
	switch v := parameters[0].(type) {
	case int, int32, int64:
		newZoneID = fmt.Sprintf("%v", v)
	case string:
		newZoneID = v
	case []byte:
		newZoneID = string(v)
	}

	if newZoneID != "unknown" && newZoneID != "0" {
		t.saveLocation(newZoneID)
	}
}

// Вспомогательный метод для красивого вывода и записи в файл
func (t *Tracker) saveLocation(newZoneID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Игнорируем дубликаты: если сервер прислал тот же ID дважды подряд (например, в маленьком и большом пакете),
	// мы не пишем переход на самого себя.
	if newZoneID == t.currentZoneID {
		return
	}

	// 1. Переводим старый и новый ID в понятные названия через базу зон data/zones.json
	oldZoneName := config.DB.GetZoneName(t.currentZoneID)
	newZoneName := config.DB.GetZoneName(newZoneID)

	// 2. Выводим красивую стрелочку в консоль программы
	log.Printf("[ПЕРЕХОД] %s ➔ %s", oldZoneName, newZoneName)

	// 3. Записываем отформатированную строку в текстовый файл
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logLine := fmt.Sprintf("%s | %s [%s] ➔ %s [%s]\n",
		timestamp, oldZoneName, t.currentZoneID, newZoneName, newZoneID)

	if _, err := t.logFile.WriteString(logLine); err != nil {
		log.Printf("Ошибка записи в файл лога: %v", err)
	}

	// 4. Обновляем текущий ID локации: теперь «новая» зона стала нашей «текущей»
	t.currentZoneID = newZoneID
}


// Close закрывает файл лога.
func (t *Tracker) Close() error {
	return t.logFile.Close()
}
