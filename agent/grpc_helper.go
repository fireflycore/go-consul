package agent

import (
	"sort"

	"google.golang.org/grpc"
)

// BuildGRPCMethods 把 gRPC ServiceDesc 集合转换成稳定有序的完整 method path 列表。
func BuildGRPCMethods(rawServices []*grpc.ServiceDesc) []string {
	// 先统计总方法数，避免后续 append 过程中多次扩容。
	totalMethods := 0
	for _, desc := range rawServices {
		// 空描述不参与统计。
		if desc == nil {
			continue
		}
		// 直接累加当前服务描述下的方法数。
		totalMethods += len(desc.Methods)
	}
	// 按精确容量一次性分配结果切片。
	methods := make([]string, 0, totalMethods)
	// 遍历所有 gRPC 服务描述。
	for _, desc := range rawServices {
		// 空描述直接跳过，避免解引用空指针。
		if desc == nil {
			continue
		}
		// 遍历当前服务描述下的所有普通方法。
		for _, method := range desc.Methods {
			// 使用字符串拼接替代格式化调用，降低热点路径上的分配和开销。
			methods = append(methods, "/"+desc.ServiceName+"/"+method.MethodName)
		}
	}
	// 对方法路径做排序，确保相同输入得到稳定输出。
	sort.Strings(methods)
	// 返回排序后的完整方法列表。
	return methods
}
