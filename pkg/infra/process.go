package infra

import (
	"fmt"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var limiter *rate.Limiter

var s1kString = "123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890" +
	"123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890123456890"

func Process(configPath string, num int, logger *log.Logger) error {
	config, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	crypto, err := config.LoadCrypto()
	if err != nil {
		return err
	}
	raw := make(chan *Elements, 500)
	signed := make([]chan *Elements, len(config.Endorsers))
	processed := make(chan *Elements, 500)
	envs := make(chan *Elements, 500)
	done := make(chan struct{})
	finishCh := make(chan struct{})
	errorCh := make(chan error, 500)
	assember := &Assembler{Signer: crypto, SignCount: config.EndorsersCount}

	limiter = rate.NewLimiter(rate.Limit(config.LimitBucket), config.LimitBucket + 100)

	for i := 0; i < len(config.Endorsers); i++ {
		signed[i] = make(chan *Elements, 10)
	}

	for i := 0; i < 5; i++ {
		go assember.StartSigner(raw, signed, errorCh, done)
		go assember.StartIntegrator(processed, envs, errorCh, done)
	}

	proposor, err := CreateProposers(config.NumOfConn, config.ClientPerConn, config.Endorsers, logger)
	if err != nil {
		return err
	}
	proposor.Start(signed, processed, done, config)

	broadcaster, err := CreateBroadcasters(config.NumOfConn, config.Orderer, logger)
	if err != nil {
		return err
	}
	broadcaster.Start(envs, errorCh, done)

	observer, err := CreateObserver(config.Channel, config.Committer, crypto, logger)
	if err != nil {
		return err
	}


	go TpsCalc(done)

	start := time.Now()
	go observer.Start(num, errorCh, finishCh, start)
	go func() {
		for i := 0; i < num; i++ {
			uuid, err := getuuid()
			if err != nil {
				logger.Errorf(err.Error())
				continue
			}
			config.Args[1] = uuid
			config.Args[2] = s1kString
			prop, err := CreateProposal(
				crypto,
				config.Channel,
				config.Chaincode,
				config.Version,
				config.Args...,
			)
			if err != nil {
				errorCh <- errors.Wrapf(err, "error creating proposal")
				return
			}
			raw <- &Elements{Proposal: prop}
		}
	}()

	for {
		select {
		case err = <-errorCh:
			return err
		case <-finishCh:
			duration := time.Since(start)
			close(done)

			logger.Infof("Completed processing transactions.")
			fmt.Printf("tx: %d, duration: %+v, tps: %f\n", num, duration, float64(num)/duration.Seconds())
			return nil
		}
	}
}

func TpsCalc(done <-chan struct{}) {
	var clientStart,clientEnd int32
	var serverStart,serverEnd int32
	for {

		time.Sleep(10 * time.Second)

		select {
		case <-done:
			return
		default:
		}

		serverEnd = BlockTxCount
		clientEnd = atomic.LoadInt32(&tpsCalc)
		fmt.Printf("tx count:%d, client tps:%v, server tps:%v\n",
			tpsCalc, (clientEnd - clientStart) / 10.0, (serverEnd - serverStart) / 10.0)
		clientStart = clientEnd
		serverStart = serverEnd
	}
}