// config/config.go
package config

// Порт игрового сервера Albion Online (UDP)
const GamePort = 5056

// BPF-фильтр для захвата только игрового трафика
const BPFFilter = "udp and port 5056"
//const BPFFilter = "udp"
// Путь к файлу, в который будут записываться переходы
const LogFilePath = "location_history.log"

// Размер буфера для канала с сырыми пакетами
const PacketChannelSize = 4096

const ApiUrl = "https://telifon-map.heliohost.us/albion-tracker-map/map/api/packet/"
