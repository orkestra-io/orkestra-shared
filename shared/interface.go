package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"net/rpc"

	"github.com/hashicorp/go-plugin"
	"github.com/orkestra-io/orkestra-shared/proto"
	"google.golang.org/grpc"
)

// HandshakeConfig est utilisé pour s'assurer que le moteur et le plugin
// communiquent sur la même version.
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "ORKESTRA_PLUGIN",
	MagicCookieValue: "hello",
}

// NodeExecutor est l'interface que tous les plugins de nœuds doivent implémenter.
type NodeExecutor interface {
	Execute(node Node, ctx ExecutionContext) (interface{}, error)
	GetCapabilities() ([]string, error)
}

type Retries struct {
	Count int    `json:"count"`
	Delay string `json:"delay"`
}

type ExecutionContext struct {
	TriggerData map[string]interface{}
	NodeOutputs map[string]interface{}
	Secrets     map[string]string
	CurrentItem interface{}
	FailureData map[string]interface{}
}

type Node struct {
	ID        string
	Uses      string
	With      map[string]interface{}
	Needs     []string
	Do        []*Node
	Retries   *Retries
	OnFailure []*Node
}

// --- gRPC Implementation ---

// NodeExecutorGRPC est le client gRPC.
type NodeExecutorGRPC struct {
	client proto.NodeExecutorClient
}

func (m *NodeExecutorGRPC) Execute(node Node, ctx ExecutionContext) (interface{}, error) {
	req, err := toProtoExecuteRequest(node, ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to convert request for gRPC: %w", err)
	}
	resp, err := m.client.Execute(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return fromProtoValue(resp.Result)
}

func (m *NodeExecutorGRPC) GetCapabilities() ([]string, error) {
	resp, err := m.client.GetCapabilities(context.Background(), &proto.Empty{})
	if err != nil {
		return nil, err
	}
	return resp.Uses, nil
}

type NodeExecutorGRPCServer struct {
	proto.UnimplementedNodeExecutorServer
	Impl NodeExecutor
}

func (s *NodeExecutorGRPCServer) Execute(ctx context.Context, req *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	node, execCtx, err := fromProtoExecuteRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to convert request from proto: %w", err)
	}

	result, err := s.Impl.Execute(node, execCtx)
	if err != nil {
		return nil, err
	}

	protoResult, err := toProtoValue(result)
	if err != nil {
		return nil, fmt.Errorf("failed to convert result to proto: %w", err)
	}

	return &proto.ExecuteResponse{Result: protoResult}, nil
}

func (s *NodeExecutorGRPCServer) GetCapabilities(ctx context.Context, req *proto.Empty) (*proto.GetCapabilitiesResponse, error) {
	uses, err := s.Impl.GetCapabilities()
	if err != nil {
		return nil, err
	}
	return &proto.GetCapabilitiesResponse{Uses: uses}, nil
}

// --- Implémentation du wrapper go-plugin ---

type NodeExecutorPlugin struct {
	plugin.GRPCPlugin
	Impl NodeExecutor
}

func (p *NodeExecutorPlugin) Server(*plugin.MuxBroker) (interface{}, error) {
	return nil, fmt.Errorf("NetRPC is not supported")
}

func (p *NodeExecutorPlugin) Client(*plugin.MuxBroker, *rpc.Client) (interface{}, error) {
	return nil, fmt.Errorf("NetRPC is not supported")
}

func (p *NodeExecutorPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterNodeExecutorServer(s, &NodeExecutorGRPCServer{Impl: p.Impl})
	return nil
}

func (p *NodeExecutorPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &NodeExecutorGRPC{client: proto.NewNodeExecutorClient(c)}, nil
}

// --- Fonctions de Conversion (Helpers) ---

func toProtoExecuteRequest(node Node, ctx ExecutionContext) (*proto.ExecuteRequest, error) {
	protoNode, err := toProtoNode(&node)
	if err != nil {
		return nil, err
	}
	protoCtx, err := toProtoExecutionContext(&ctx)
	if err != nil {
		return nil, err
	}
	return &proto.ExecuteRequest{Node: protoNode, Context: protoCtx}, nil
}

func fromProtoExecuteRequest(req *proto.ExecuteRequest) (Node, ExecutionContext, error) {
	node, err := fromProtoNode(req.Node)
	if err != nil {
		return Node{}, ExecutionContext{}, err
	}
	execCtx, err := fromProtoExecutionContext(req.Context)
	if err != nil {
		return Node{}, ExecutionContext{}, err
	}
	return node, execCtx, nil
}

func toProtoNode(node *Node) (*proto.Node, error) {
	if node == nil {
		return nil, nil
	}
	with, err := json.Marshal(node.With)
	if err != nil {
		return nil, err
	}

	var doNodes []*proto.Node
	for _, doNode := range node.Do {
		pn, err := toProtoNode(doNode)
		if err != nil {
			return nil, err
		}
		doNodes = append(doNodes, pn)
	}

	retries, err := json.Marshal(node.Retries)
	if err != nil {
		return nil, err
	}

	var onFailureNodes []*proto.Node
	for _, failNode := range node.OnFailure {
		pn, err := toProtoNode(failNode)
		if err != nil {
			return nil, err
		}
		onFailureNodes = append(onFailureNodes, pn)
	}

	return &proto.Node{
		Id:        node.ID,
		Uses:      node.Uses,
		With:      with,
		Needs:     node.Needs,
		Do:        doNodes,
		Retries:   retries,
		OnFailure: onFailureNodes,
	}, nil
}

func toProtoExecutionContext(ctx *ExecutionContext) (*proto.ExecutionContext, error) {
	triggerData, err := json.Marshal(ctx.TriggerData)
	if err != nil {
		return nil, err
	}
	nodeOutputs, err := json.Marshal(ctx.NodeOutputs)
	if err != nil {
		return nil, err
	}
	currentItem, err := json.Marshal(ctx.CurrentItem)
	if err != nil {
		return nil, err
	}
	failureData, err := json.Marshal(ctx.FailureData)
	if err != nil {
		return nil, err
	}

	return &proto.ExecutionContext{
		TriggerData: triggerData,
		NodeOutputs: nodeOutputs,
		Secrets:     ctx.Secrets,
		CurrentItem: currentItem,
		FailureData: failureData,
	}, nil
}

func toProtoValue(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func fromProtoNode(pNode *proto.Node) (Node, error) {
	if pNode == nil {
		return Node{}, nil
	}
	var with map[string]interface{}
	if err := json.Unmarshal(pNode.With, &with); err != nil {
		return Node{}, err
	}

	var doNodes []*Node
	for _, pDoNode := range pNode.Do {
		dn, err := fromProtoNode(pDoNode)
		if err != nil {
			return Node{}, err
		}
		doNodes = append(doNodes, &dn)
	}

	var retries *Retries
	if len(pNode.Retries) > 0 && string(pNode.Retries) != "null" {
		if err := json.Unmarshal(pNode.Retries, &retries); err != nil {
			return Node{}, err
		}
	}

	var onFailureNodes []*Node
	for _, pFailNode := range pNode.OnFailure {
		fn, err := fromProtoNode(pFailNode)
		if err != nil {
			return Node{}, err
		}
		onFailureNodes = append(onFailureNodes, &fn)
	}

	return Node{
		ID:        pNode.Id,
		Uses:      pNode.Uses,
		With:      with,
		Needs:     pNode.Needs,
		Do:        doNodes,
		Retries:   retries,
		OnFailure: onFailureNodes,
	}, nil
}

func fromProtoExecutionContext(pCtx *proto.ExecutionContext) (ExecutionContext, error) {
	var triggerData, nodeOutputs, currentItem, failureData map[string]interface{}
	if len(pCtx.TriggerData) > 0 {
		if err := json.Unmarshal(pCtx.TriggerData, &triggerData); err != nil {
			return ExecutionContext{}, err
		}
	}
	if len(pCtx.NodeOutputs) > 0 {
		if err := json.Unmarshal(pCtx.NodeOutputs, &nodeOutputs); err != nil {
			return ExecutionContext{}, err
		}
	}
	if len(pCtx.CurrentItem) > 0 {
		if err := json.Unmarshal(pCtx.CurrentItem, &currentItem); err != nil {
			return ExecutionContext{}, err
		}
	}
	if len(pCtx.FailureData) > 0 {
		if err := json.Unmarshal(pCtx.FailureData, &failureData); err != nil {
			return ExecutionContext{}, err
		}
	}

	return ExecutionContext{
		TriggerData: triggerData,
		NodeOutputs: nodeOutputs,
		Secrets:     pCtx.Secrets,
		CurrentItem: currentItem,
		FailureData: failureData,
	}, nil
}

func fromProtoValue(b []byte) (interface{}, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return v, nil
}
