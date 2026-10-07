// photon/parser.go
package photon

import (

	//"log"

	//"fmt"

	photon "github.com/AutoDruid/photon-parser"
)

// EventHandler вызывается для каждого события в распарсенном пакете.
type EventHandler func(code uint8, parameters []interface{})
type ResponseHandler func(opCode uint8, parameters []interface{})
// Parser оборачивает Photon-парсер и вызывает обработчик для событий.
type Parser struct {
	onEvent EventHandler
	onResponse ResponseHandler
}

// NewParser создаёт новый парсер с заданным обработчиком событий.
func NewParser(onEvent EventHandler, onResponse ResponseHandler) *Parser {
	return &Parser{
		onEvent: onEvent,
		onResponse: onResponse,
	}
}

// ParsePayload принимает сырые байты UDP payload и парсит их.
func (p *Parser) ParsePayload(payload []byte) {
	parser := photon.NewParserV18()
	var sess photon.SessionV18
	if err := parser.ParsePacketInto(payload, &sess); err != nil {
		return
	}
	for _, cmd := range sess.Commands {
		maintype := cmd.CommandHeader.Type
		if maintype != 6 {
			continue
		}
		header := cmd.ReliablePayload.ReliableHeader
		intertype := header.Type
		eventcode := header.EventCode
		params := cmd.ReliablePayload.Parameters
		switch intertype{
		case 3:
			if p.onResponse != nil{
				p.onResponse(eventcode, p.extract(params) )
			}
		case 4:
			if p.onEvent != nil{
				p.onEvent(eventcode, p.extract(params))
			}
		}
		if intertype != 3 { continue }
		//code := header.EventCode
		//if code == 1 {continue}
		//log.Printf("---------------------------------CMD: %+v %s", cmd, "--------------------------------------------")
	} 
}

		// Вспомогательная функция для распаковки параметров библиотеки в []interface{}
func (p *Parser) extract(params interface{}) []interface{} {
	var cleanParams []interface{}
	switch v := params.(type) {
	case []photon.ParameterV18:
		for _, item := range v {
			if len(item.Value.Blob) > 0 {
				cleanParams = append(cleanParams, item.Value.Blob)
			} else {
				cleanParams = append(cleanParams, item.Value.Num)
			}
		}
	}
	return cleanParams
}