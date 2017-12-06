package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	common "github.com/runconduit/conduit/controller/gen/common"
	pb "github.com/runconduit/conduit/controller/gen/public"
	"github.com/runconduit/conduit/controller/util"
	"github.com/spf13/cobra"
)

var (
	maxRps    float32
	toPort    uint32
	toIP      string
	fromPort  uint32
	fromIP    string
	scheme    string
	method    string
	authority string
	path      string
)

var tapCmd = &cobra.Command{
	Use:   "tap [flags] (deployment|pod) TARGET",
	Short: "Listen to a traffic stream",
	Long: `Listen to a traffic stream.

Valid targets include:
 * Pods (default/hello-world-h4fb2)
 * Deployments (default/hello-world)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch len(args) {
		case 2:
			resourceType := strings.ToLower(args[0])

			// We don't validate inputs because they are validated on the server.
			req := &pb.TapRequest{
				MaxRps:    maxRps,
				ToPort:    toPort,
				ToIP:      toIP,
				FromPort:  fromPort,
				FromIP:    fromIP,
				Scheme:    scheme,
				Method:    method,
				Authority: authority,
				Path:      path,
			}

			switch resourceType {
			case "deploy", "deployment", "deployments":
				req.Target = &pb.TapRequest_Deployment{
					Deployment: args[1],
				}

			case "po", "pod", "pods":
				req.Target = &pb.TapRequest_Pod{
					Pod: args[1],
				}

			default:
				return errors.New("invalid target type")
			}

			client, err := newApiClient()
			if err != nil {
				return err
			}
			rsp, err := client.Tap(context.Background(), req)
			if err != nil {
				return err
			}
			print(rsp)

			return nil
		default:
			return errors.New("please specify a target")
		}
	},
}

func init() {
	RootCmd.AddCommand(tapCmd)
	addControlPlaneNetworkingArgs(tapCmd)
	tapCmd.PersistentFlags().Float32Var(&maxRps, "max-rps", 1.0, "Maximum requests per second to tap.")
	tapCmd.PersistentFlags().Uint32Var(&toPort, "to-port", 0, "Display requests to this port")
	tapCmd.PersistentFlags().StringVar(&toIP, "to-ip", "", "Display requests to this IP")
	tapCmd.PersistentFlags().Uint32Var(&fromPort, "from-port", 0, "Display requests from this port")
	tapCmd.PersistentFlags().StringVar(&fromIP, "from-ip", "", "Display requests from this IP")
	tapCmd.PersistentFlags().StringVar(&scheme, "scheme", "", "Display requests with this scheme")
	tapCmd.PersistentFlags().StringVar(&method, "method", "", "Display requests with this HTTP method")
	tapCmd.PersistentFlags().StringVar(&authority, "authority", "", "Display requests with this :authority")
	tapCmd.PersistentFlags().StringVar(&path, "path", "", "Display requests with paths that start with this prefix")
}

func print(rsp pb.Api_TapClient) {
	for {
		event, err := rsp.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println(err)
			break
		}

		flow := fmt.Sprintf("src=%s dst=%s",
			util.AddressToString(event.GetSource()),
			util.AddressToString(event.GetTarget()),
		)

		switch ev := event.GetHttp().Event.(type) {
		case *common.TapEvent_Http_RequestInit_:
			fmt.Printf("req id=%d:%d %s :method=%s :authority=%s :path=%s\n",
				ev.RequestInit.Id.Base,
				ev.RequestInit.Id.Stream,
				flow,
				ev.RequestInit.Method.GetRegistered().String(),
				ev.RequestInit.Authority,
				ev.RequestInit.Path,
			)
		case *common.TapEvent_Http_ResponseInit_:
			fmt.Printf("rsp id=%d:%d %s :status=%d latency=%dµs\n",
				ev.ResponseInit.Id.Base,
				ev.ResponseInit.Id.Stream,
				flow,
				ev.ResponseInit.GetHttpStatus(),
				ev.ResponseInit.GetSinceRequestInit().Nanos/1000,
			)
		case *common.TapEvent_Http_ResponseEnd_:
			fmt.Printf("end id=%d:%d %s grpc-status=%s duration=%dµs response-length=%dB\n",
				ev.ResponseEnd.Id.Base,
				ev.ResponseEnd.Id.Stream,
				flow,
				grpcStatusName(ev.ResponseEnd.GetGrpcStatus()),
				ev.ResponseEnd.GetSinceResponseInit().Nanos/1000,
				ev.ResponseEnd.GetResponseBytes(),
			)
		}
	}
}

func grpcStatusName(n uint32) string {
	switch n {
	case 0:
		return "OK"
	case 1:
		return "CANCELED"
	case 2:
		return "UNKNOWN"
	case 12:
		return "UNIMPLEMENTED"
	case 13:
		return "INTERNAL"
	case 14:
		return "UNAVAILABLE"
	// TODO ...
	default:
		return "???"
	}
}
