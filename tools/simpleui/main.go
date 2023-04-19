package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	pb "github.com/tektoncd/results/proto/v1alpha2/results_go_proto"
	"google.golang.org/grpc"
)

var (
	resultTmpl = template.Must(template.New("results.html").ParseFiles("templates/results.html"))
	recordTmpl = template.Must(template.New("records.html").
			Funcs(template.FuncMap{
			"textproto":  textproto,
			"parent":     parent,
			"trimprefix": strings.TrimPrefix,
		}).
		ParseFiles("templates/records.html"))
)

func (u *ui) home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Add namespace to URL (/<namespace>) ^^^")
}

func (u *ui) results(w http.ResponseWriter, r *http.Request) {
	res, err := u.client.ListResults(r.Context(), &pb.ListResultsRequest{
		Parent: strings.Trim(r.URL.Path, "/"),
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, err)
		return
	}
	if err := resultTmpl.Execute(w, res); err != nil {
		return
	}
}

func (u *ui) records(w http.ResponseWriter, r *http.Request) {
	req := &pb.ListRecordsRequest{
		Parent: strings.Trim(r.URL.Path, "/"),
		Filter: r.FormValue("query"),
	}
	res, err := u.client.ListRecords(r.Context(), req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, err)
		return
	}
	if err := recordTmpl.Execute(w, struct {
		Request  *pb.ListRecordsRequest
		Response *pb.ListRecordsResponse
	}{
		Request:  req,
		Response: res,
	}); err != nil {
		return
	}
}

type ui struct {
	client pb.ResultsClient
}

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	u := &ui{
		client: pb.NewResultsClient(conn),
	}

	r := mux.NewRouter()
	r.HandleFunc("/", u.home)
	r.HandleFunc("/{namespace}", u.results)
	r.HandleFunc("/{namespace}/results/{name}", u.records)
	http.Handle("/", r)

	log.Println("Running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
