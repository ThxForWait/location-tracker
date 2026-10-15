// config/config.go
package config

import "time"

// Порт игрового сервера Albion Online (UDP)
const GamePort = 5056

// BPF-фильтр для захвата только игрового трафика
const BPFFilter = "udp and port 5056"
//const BPFFilter = "udp"
// Путь к файлу, в который будут записываться переходы
const LogFilePath = "location_history.log"

// Размер буфера для канала с сырыми пакетами
const PacketChannelSize = 4096

// Базовые адреса API v1 (пути /edges/..., /device/... добавляет api.Client)
const DebugApiUrl = "http://127.0.0.1:8000/api/v1" // только для дебаг (флаг -debug)
const ApiUrl = "https://telifon-map.heliohost.us/albion-tracker-map/api/v1"

// Версия трекера, уходит на сервер в X-Client-Version.
// Задаётся при сборке: go build -ldflags "-X location-tracker/config.Version=1.0.0"
var Version = "dev"

// Папка настроек в %APPDATA% (там хранится токен устройства)
const AppDirName = "AlbionTracker"

// Время жизни портала Авалона, которое отправляется на сервер.
// Пока фиксированное значение для теста: реального таймера в пакетах не найдено.
const PortalLifetime = 2 * time.Hour

// Сколько активный интерфейс должен молчать, чтобы переключиться на другой
// (например, когда игрок включил VPN).
const InterfaceSwitchAfter = 3 * time.Second

// Как часто искать новые интерфейсы (VPN-адаптеры появляются при подключении).
const InterfaceRescanInterval = 10 * time.Second

// Размер очереди переходов, ожидающих отправки на сервер.
const SendQueueSize = 256

// Переход старше этого возраста больше не отправляется (сервер его всё равно отвергнет).
const SendMaxAge = 5 * time.Minute

// Сколько ждать досылки очереди при выходе по Ctrl+C.
const SendShutdownGrace = 3 * time.Second
