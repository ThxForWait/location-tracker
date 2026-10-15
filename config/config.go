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

const DebugApiUrl = "http://127.0.0.1:8000/map/api/packet/" // только для дебаг (флаг -debug)
const ApiUrl = "https://telifon-map.heliohost.us/albion-tracker-map/map/api/packet/"

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
