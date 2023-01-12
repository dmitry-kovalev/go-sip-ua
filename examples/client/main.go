package main

import (
	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/dmitry-kovalev/go-sip-ua/pkg/account"
	"github.com/dmitry-kovalev/go-sip-ua/pkg/media/rtp"
	"github.com/dmitry-kovalev/go-sip-ua/pkg/session"
	"github.com/dmitry-kovalev/go-sip-ua/pkg/stack"
	"github.com/dmitry-kovalev/go-sip-ua/pkg/ua"
	"github.com/dmitry-kovalev/go-sip-ua/pkg/utils"
	"github.com/ghettovoice/gosip/log"
	"github.com/ghettovoice/gosip/sip"
	"github.com/ghettovoice/gosip/sip/parser"
	"github.com/sirupsen/logrus"
	"net"
	"os"
	"os/signal"
	"syscall"
)

var (
	logger log.Logger
	udp    *rtp.RtpUDPStream
)

func createUdp() *rtp.RtpUDPStream {

	udp = rtp.NewRtpUDPStream("127.0.0.1", rtp.DefaultPortMin, rtp.DefaultPortMax, func(data []byte, raddr net.Addr) {
		logger.Infof("Rtp recevied: %v, laddr %s : raddr %s", len(data), udp.LocalAddr().String(), raddr)
		dest, _ := net.ResolveUDPAddr(raddr.Network(), raddr.String())
		logger.Infof("Echo rtp to %v", raddr)
		udp.Send(data, dest)
	})

	go udp.Read()

	return udp
}

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	customLogger := logrus.New()
	customLogger.SetFormatter(&nested.Formatter{
		HideKeys:        true,
		FieldsOrder:     []string{"component"},
		NoColors:        false,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	customLogger.SetLevel(logrus.InfoLevel)
	logger = log.NewLogrusLogger(customLogger, "Client", nil)
	loggers := make(map[string]*utils.MyLogger)
	loggers["SipStack"] = &utils.MyLogger{
		Logger: log.NewLogrusLogger(customLogger, "SipStack", nil),
	}
	loggers["transport.Layer"] = &utils.MyLogger{
		Logger: log.NewLogrusLogger(customLogger, "transport.Layer", nil),
	}
	loggers["transaction.Layer"] = &utils.MyLogger{
		Logger: log.NewLogrusLogger(customLogger, "transaction.Layer", nil),
	}
	loggers["UserAgent"] = &utils.MyLogger{
		Logger: log.NewLogrusLogger(customLogger, "UserAgent", nil),
	}
	utils.SetLoggers(loggers)
	stack := stack.NewSipStack(&stack.SipStackConfig{
		UserAgent:  "Go Sip Client/example-client",
		Extensions: []string{"replaces", "outbound"},
		Dns:        "8.8.8.8"})

	listen := "0.0.0.0:5080"
	logger.Infof("Listen => %s", listen)

	if err := stack.Listen("udp", listen); err != nil {
		logger.Panic(err)
	}
	//
	// if err := stack.Listen("tcp", listen); err != nil {
	// 	logger.Panic(err)
	// }
	//
	// if err := stack.ListenTLS("wss", "0.0.0.0:5091", nil); err != nil {
	// 	logger.Panic(err)
	// }

	ua := ua.NewUserAgent(&ua.UserAgentConfig{
		SipStack: stack,
	})

	ua.InviteStateHandler = func(sess *session.Session, req *sip.Request, resp *sip.Response, state session.Status) {
		logger.Infof("InviteStateHandler: state => %v, type => %s", state, sess.Direction())

		switch state {
		case session.InviteReceived:
			// udp = createUdp()
			// udpLaddr := udp.LocalAddr()
			// sdp := mock.BuildLocalSdp(udpLaddr.IP.String(), udpLaddr.Port)
			// sess.ProvideAnswer(sdp)
			// sess.Accept(200)
			sess.Provisional(180, "ringing")
		case session.Canceled:
			fallthrough
		case session.Failure:
			fallthrough
		case session.Terminated:
		}
	}

	ua.RegisterStateHandler = func(state account.RegisterState) {
		logger.Infof("RegisterStateHandler: user => %s, state => %v, expires => %v", state.Account.AuthInfo.AuthUser, state.StatusCode, state.Expiration)
	}

	uri, err := parser.ParseUri("sip:703@ai001093.aicall.ru")
	if err != nil {
		logger.Error(err)
	}

	profile := account.NewProfile(uri.Clone(), "goSIP/example-client",
		&account.AuthInfo{
			AuthUser: "703",
			Password: "secret_password",
			Realm:    "my.sip.domain",
		},
		1800,
		stack,
	)

	recipient, err := parser.ParseSipUri("sip:my.sip.domain:5060")
	if err != nil {
		logger.Error(err)
	}

	register, _ := ua.SendRegister(profile, recipient, profile.Expires, nil)
	// time.Sleep(time.Second * 3)

	// udp = createUdp()
	// udpLaddr := udp.LocalAddr()
	// sdp := mock.BuildLocalSdp(udpLaddr.IP.String(), udpLaddr.Port)
	//
	// called, err2 := parser.ParseUri("sip:400@127.0.0.1")
	// if err2 != nil {
	// 	logger.Error(err)
	// }
	//
	// recipient, err = parser.ParseSipUri("sip:400@127.0.0.1:5081;transport=wss")
	// if err != nil {
	// 	logger.Error(err)
	// }
	//
	// go ua.Invite(profile, called, recipient, &sdp)

	<-stop

	register.SendRegister(0)

	ua.Shutdown()
}
