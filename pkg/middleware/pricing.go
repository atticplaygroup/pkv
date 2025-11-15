package middleware

import (
	"fmt"

	"connectrpc.com/connect"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	pbconnect "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1/kvstoreconnect"
)

type IPricingManager interface {
	GetPrice(req connect.AnyRequest) (int64, error)
}

type PricingManager struct {
}

// TODO: protect against too large values eating memory
func (p *PricingManager) GetPrice(req connect.AnyRequest) (int64, error) {
	BYTE_PRICE := 1
	STREAM_BYTE_PRICE := 100
	switch req.Spec().Procedure {
	case pbconnect.KvStoreServiceRegisterInstanceProcedure:
		CID_PRICE := 1
		if r, ok := req.Any().(*pb.RegisterInstanceRequest); !ok {
			return 0, fmt.Errorf("failed to parse request")
		} else {
			return int64(CID_PRICE * len(r.GetAdvertisement().GetCids())), nil
		}
	case pbconnect.KvStoreServiceCreateValueProcedure:
		if r, ok := req.Any().(*pb.CreateValueRequest); !ok {
			return 0, fmt.Errorf("failed to parse request")
		} else {
			// TODO: use reader for large objects
			return int64(BYTE_PRICE * len(r.GetValue())), nil
		}
	case pbconnect.KvStoreServiceProlongValueProcedure:
		if r, ok := req.Any().(*pb.ProlongValueRequest); !ok {
			return 0, fmt.Errorf("failed to parse request")
		} else {
			return int64(BYTE_PRICE) * r.GetMaxSize(), nil
		}
	case pbconnect.KvStoreServiceCreateStreamValueProcedure:
		if r, ok := req.Any().(*pb.CreateStreamValueRequest); !ok {
			return 0, fmt.Errorf("failed to parse request")
		} else {
			return int64(STREAM_BYTE_PRICE) * int64(len(r.GetValue())), nil
		}
	default:
		return 1, nil
	}
}
