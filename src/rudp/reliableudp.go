package rudp

import "udp"
import "github.com/woodywanghg/gofclog"
import "github.com/golang/protobuf/proto"
import "rudpproto"
import "time"
import "sync"
import "errors"

type ReliableUdp struct {
	encrypt    RudpEncrypt
	udpSocket  udpsocket.UdpSocket
	lock       sync.Mutex
	sessionMap map[int64]*UdpSession
}

func (r *ReliableUdp) Listen(ip string, port int) error {

	r.encrypt.Init()
	r.sessionMap = make(map[int64]*UdpSession, 0)
	r.udpSocket.SetUdpReceiver(r)

	err := r.udpSocket.Listen(ip, port)
	if err != nil {
		fclog.ERROR("ReliableUdp init error! err=%s", err.Error())
		return err
	}

	return nil
}

func (r *ReliableUdp) OnUdpRecv(b []byte, bLen int, ip string, port int) {
	tempBuf := b[0:bLen]
	fclog.DEBUG("Recv data=%d byte=%s", bLen, string(tempBuf))

	if !r.encrypt.IsValidPacket(tempBuf) {
		fclog.ERROR("Invalid packet")
		return
	}

	packetData := r.encrypt.GetPacketData(tempBuf)

	r.DecodePacket(packetData, ip, port)

	//r.udpSocket.SendData(packetData, ip, port)

	fclog.DEBUG("Send ip=%s port=%d packetData=%v", ip, port, packetData)
}

func (r *ReliableUdp) DecodePacket(b []byte, ip string, port int) {

	var msg rudpmsg.RudpMessage

	err := proto.Unmarshal(b, &msg)
	if err != nil {
		fclog.ERROR("Unmarshal error! err=%s", err.Error())
		return
	}

	var msgType rudpmsg.RudpMsgType

	switch {

	case msgType == rudpmsg.RudpMsgType_MSG_RUDP_DATA:
		r.ProcessMsgData(msg.Data, ip, port)
	case msgType == rudpmsg.RudpMsgType_MSG_RUDP_ACK:
		r.ProcessMsgAck(msg.Data, ip, port)
	case msgType == rudpmsg.RudpMsgType_MSG_RUDP_REG:
		r.ProcessMsgReg(msg.Data, ip, port)
	case msgType == rudpmsg.RudpMsgType_MSG_RUDP_REG_RS:
		r.ProcessMsgRegRs(msg.Data, ip, port)
	}

}

func (r *ReliableUdp) ProcessMsgData(b []byte, ip string, port int) {

	var msgData rudpmsg.RudpMsgData
	err := proto.Unmarshal(b, &msgData)

	if err != nil {
		fclog.ERROR("Unmarshal error! err=%s", err.Error())
		return
	}

}

func (r *ReliableUdp) ProcessMsgAck(b []byte, ip string, port int) {
	var msgData rudpmsg.RudpMsgAck
	err := proto.Unmarshal(b, &msgData)

	if err != nil {
		fclog.ERROR("Unmarshal error! err=%s", err.Error())
		return
	}

	sid := int64(*msgData.Sid)
	seq := int64(*msgData.Seq)

	r.lock.Lock()
	defer r.lock.Unlock()

	udpSession, exist := r.sessionMap[sid]
	if !exist {
		fclog.ERROR("Receive invalid data sid=%d seq=%d", sid, seq)
		return
	}

	udpSession.OnAck(seq)
}

func (r *ReliableUdp) ProcessMsgReg(b []byte, ip string, port int) {
	var msgData rudpmsg.RudpMsgReg
	err := proto.Unmarshal(b, &msgData)

	if err != nil {
		fclog.ERROR("Unmarshal error! err=%s", err.Error())
		return
	}

	sid := int64(*msgData.Sid)
	seq := int64(*msgData.Sid)

	r.lock.Lock()
	_, exist := r.sessionMap[sid]
	r.lock.Unlock()

	if exist {
		fclog.ERROR("Register error!, exist sessioin id! id=%d", sid)
		r.SendInvalidSessionRs(sid, ip, port)
		return
	}

	var udpSession *UdpSession = new(UdpSession)
	udpSession.Init(sid, ip, port, &r.udpSocket, r)
	r.sessionMap[sid] = udpSession

	r.sendAck(udpSession, seq)
	r.SendCreateSessionRs(udpSession)

}

func (r *ReliableUdp) ProcessMsgRegRs(b []byte, ip string, port int) {
	var msgData rudpmsg.RudpMsgRegRs
	err := proto.Unmarshal(b, &msgData)

	if err != nil {
		fclog.ERROR("Unmarshal error! err=%s", err.Error())
		return
	}

	sid := int64(*msgData.Sid)
	seq := int64(*msgData.Seq)
	code := int64(*msgData.Code)

	r.lock.Lock()
	defer r.lock.Unlock()

	udpSession, exist := r.sessionMap[sid]
	if !exist {
		fclog.ERROR("Receive invalid data sid=%d seq=%d", sid, seq)
		return
	}

	udpSession.OnAck(seq)

	fclog.DEBUG("Receive udp session response sessionid=%d code=%d", sid, code)
}

func (r *ReliableUdp) CreateSessionListen(ip string, port int) (int64, error) {

	Sid := time.Now().UnixNano()

	var msg rudpmsg.RudpMsgReg
	msg.Seq = proto.Int64(0)
	msg.Sid = proto.Int64(Sid)

	data, err := proto.Marshal(&msg)
	if err != nil {
		fclog.ERROR("Marshal message error!")
		return 0, err
	}

	packetData := r.EncodePacket(data, rudpmsg.RudpMsgType_MSG_RUDP_REG)

	if len(packetData) <= 0 {
		fclog.ERROR("EncodePacket error!")
		return 0, err
	}

	var udpSession *UdpSession = new(UdpSession)
	udpSession.Init(Sid, ip, port, &r.udpSocket, r)

	r.lock.Lock()
	r.sessionMap[Sid] = udpSession
	r.lock.Unlock()

	encryptData := r.encrypt.EncodePacket(packetData)

	udpSession.SendData(encryptData)

	return Sid, nil
}

func (r *ReliableUdp) CreateSessionDial(ip string, port int) (int64, error) {

	Sid := time.Now().UnixNano()

	var msg rudpmsg.RudpMsgReg
	msg.Seq = proto.Int64(0)
	msg.Sid = proto.Int64(Sid)

	data, err := proto.Marshal(&msg)
	if err != nil {
		fclog.ERROR("Marshal message error!")
		return 0, err
	}

	packetData := r.EncodePacket(data, rudpmsg.RudpMsgType_MSG_RUDP_REG)

	if len(packetData) <= 0 {
		fclog.ERROR("EncodePacket error!")
		return 0, err
	}

	var udpSession *UdpSession = new(UdpSession)
	if !udpSession.Init(Sid, ip, port, nil, r) {
		return 0, errors.New("udpsession ini error!")
	}

	r.lock.Lock()
	r.sessionMap[Sid] = udpSession
	r.lock.Unlock()

	encryptData := r.encrypt.EncodePacket(packetData)

	udpSession.SendData(encryptData)

	return Sid, nil
}

func (r *ReliableUdp) EncodePacket(b []byte, msgType rudpmsg.RudpMsgType) []byte {

	var packet rudpmsg.RudpMessage
	packet.Type = &msgType
	packet.Data = b

	packetData, err := proto.Marshal(&packet)

	if err != nil {
		return make([]byte, 0)
	}

	return packetData
}

func (r *ReliableUdp) SendCreateSessionRs(udpSession *UdpSession) {

	var msg rudpmsg.RudpMsgRegRs
	msg.Seq = proto.Int64(0)
	msg.Sid = proto.Int64(udpSession.GetSid())
	msg.Code = proto.Int64(0)

	data, err := proto.Marshal(&msg)
	if err != nil {
		fclog.ERROR("Marshal message error!")
		return
	}

	packetData := r.EncodePacket(data, rudpmsg.RudpMsgType_MSG_RUDP_REG_RS)

	if len(packetData) <= 0 {
		fclog.ERROR("EncodePacket error!")
		return
	}

	encryptData := r.encrypt.EncodePacket(packetData)
	udpSession.SendData(encryptData)
}

func (r *ReliableUdp) SendInvalidSessionRs(sid int64, ip string, port int) {

	var msg rudpmsg.RudpMsgRegRs
	msg.Seq = proto.Int64(0)
	msg.Sid = proto.Int64(sid)
	msg.Code = proto.Int64(10001)

	data, err := proto.Marshal(&msg)
	if err != nil {
		fclog.ERROR("Marshal message error!")
		return
	}

	packetData := r.EncodePacket(data, rudpmsg.RudpMsgType_MSG_RUDP_REG_RS)

	if len(packetData) <= 0 {
		fclog.ERROR("EncodePacket error!")
		return
	}

	encryptData := r.encrypt.EncodePacket(packetData)
	r.udpSocket.SendData(encryptData, ip, port)
}

func (r *ReliableUdp) sendAck(udpSession *UdpSession, seq int64) {

	var msg rudpmsg.RudpMsgAck
	msg.Seq = proto.Int64(seq)
	msg.Sid = proto.Int64(udpSession.GetSid())

	data, err := proto.Marshal(&msg)
	if err != nil {
		fclog.ERROR("Marshal message error!")
		return
	}

	packetData := r.EncodePacket(data, rudpmsg.RudpMsgType_MSG_RUDP_ACK)

	if len(packetData) <= 0 {
		fclog.ERROR("EncodePacket error!")
		return
	}

	encryptData := r.encrypt.EncodePacket(packetData)
	udpSession.SendData(encryptData)
}
