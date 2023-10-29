package GbankRPC

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

// MethodType 封装可调用的方法
type MethodType struct {
	Method    reflect.Method
	ArgType   reflect.Type
	ReplyType reflect.Type
	callNums  uint64
}

// NewArg 根据 ArgType 类型创建arg
func (m *MethodType) NewArg() reflect.Value {
	var arg reflect.Value
	if m.ArgType.Kind() == reflect.Ptr {
		arg = reflect.New(m.ArgType.Elem())
	} else {
		arg = reflect.New(m.ArgType).Elem()
	}
	return arg
}

// NewReply 根据 ReplyType 类型创建reply
func (m *MethodType) NewReply() reflect.Value {
	// ReplyType 必须是指针类型
	reply := reflect.New(m.ReplyType.Elem())

	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		reply.Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		reply.Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return reply
}

// Service 服务实例
type Service struct {
	name    string
	typ     reflect.Type
	obj     reflect.Value
	methods map[string]*MethodType
}

// NewService 通过传入的 struct 实例初始化service
func NewService(obj interface{}) *Service {
	res := &Service{
		// 无法确定传入的 obj 是值类型还是指针类型，这里调用 Indirect 提取实例对象再获取名称
		name:    reflect.Indirect(reflect.ValueOf(obj)).Type().Name(),
		typ:     reflect.TypeOf(obj),
		obj:     reflect.ValueOf(obj),
		methods: make(map[string]*MethodType),
	}

	if !ast.IsExported(res.name) {
		log.Printf("NewService %s is not a valid service Name\n", res.name)
	}

	res.registerMethods()
	return res
}

// 注册 service 中的可导出、内置方法
func (s *Service) registerMethods() {
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)

		// 校验方法签名
		if method.Type.NumIn() != 3 || method.Type.NumOut() != 1 {
			log.Printf("registerMethods %s Method signature is not valid\n", method.Name)
			continue
		}
		if method.Type.In(2).Kind() != reflect.Ptr {
			log.Printf("registerMethods %s Method signature is not valid,in 2th should ptr\n", method.Name)
			continue
		}

		if method.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			log.Printf("registerMethods %s Method signature is not valid,out should error\n", method.Name)
			continue
		}

		// 判断两个入参是否是可导出方法或内置方法
		argType, replyType := method.Type.In(1), method.Type.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			log.Printf("registerMethods %s Method signature is not valid,in should be exported or builtin\n",
				method.Name)
			continue
		}

		// 方法签名校验通过
		methodType := &MethodType{
			Method:    method,
			ArgType:   argType,
			ReplyType: replyType,
			callNums:  uint64(0),
		}
		s.methods[method.Name] = methodType
	}
}

// 判断 t 是否是导出或内置类型
func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

// Call 调用指定方法
func (s *Service) Call(m *MethodType, arg, reply reflect.Value) error {
	atomic.AddUint64(&m.callNums, 1)
	f := m.Method.Func
	// 调用结构体的方法，Call的第一个参数需要是结构体实例，若是普通的方法则直接按序传入参数即可
	returnValues := f.Call([]reflect.Value{s.obj, arg, reply})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}

// GetCallNums 获取方法调用次数
func (m *MethodType) GetCallNums() uint64 {
	return atomic.LoadUint64(&m.callNums)
}
