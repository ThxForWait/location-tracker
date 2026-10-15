go build -o tracker.exe .
.\tracker.exe -iface "\Device\NPF_{BA37EF87-3C77-4A63-AC4F-FA7020FB1D19}"
.\tracker.exe -iface "\Device\NPF_{DB484304-DB04-6AA0-A33D-7236836B364D}"
.\tracker.exe -debug              # отправка на локальный сервер 127.0.0.1:8000
.\tracker.exe -api "https://..."   # свой URL API
.\tracker.exe -list               # список интерфейсов
