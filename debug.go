package GbankRPC

import (
	"fmt"
	"html/template"
	"net/http"
)

const debugText = `<html>
	<body>
	<title>GBankRPC Services</title>
	{{range .}}
	<hr>
	Service {{.Name}}
	<hr>
		<table>
		<th align=center>Method</th><th align=center>Calls</th>
		{{range $Name, $mtype := .Method}}
			<tr>
			<td align=left font=fixed>{{$Name}}({{$mtype.ArgType}}, {{$mtype.ReplyType}}) error</td>
			<td align=center>{{$mtype.CallNums}}</td>
			</tr>
		{{end}}
		</table>
	{{end}}
	</body>
	</html>`

var debug = template.Must(template.New("RPC debug").Parse(debugText))

type debugServer struct {
	*server
}
type debugService struct {
	Name   string
	Method map[string]*MethodType
}

func (d *debugServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var services []debugService
	d.serviceTable.Range(func(key, val interface{}) bool {
		service := val.(*Service)
		services = append(services, debugService{
			Name:   key.(string),
			Method: service.methods,
		})
		return true
	})

	if err := debug.Execute(w, services); err != nil {
		fmt.Fprintln(w, "rpc: error executing template:", err.Error())
	}
}
