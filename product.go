/*
 *
 * Copyright 2015 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

//go:generate protoc -I ../helloworld --go_out=plugins=grpc:../helloworld ../helloworld/helloworld.proto

// Package main implements a server for Greeter service.
package main

import (
	"context"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	jsoniter "github.com/json-iterator/go"
	pb "github.com/kinsprite/producttest/pb"

	"go.elastic.co/apm"
	"go.elastic.co/apm/module/apmgrpc"
	"go.elastic.co/apm/module/apmhttp"
	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/grpc"
)

const (
	port = ":8080"
)

var userServerURL = "http://user-test:80"

var tracingClient = apmhttp.WrapClient(http.DefaultClient)
var json = jsoniter.ConfigCompatibleWithStandardLibrary

type userHandler struct{}

func (handler *userHandler) OnCreating(userInfo *UserInfo) error {
	return giveUserFreeBooks(userInfo)
}

// server is used to implement helloworld.GreeterServer.
type server struct{}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	log.Printf("Received: %v", in.Name)
	userInfo := getUserInfo(ctx)
	userID := strconv.FormatInt(int64(userInfo.ID), 10)
	var books string

	if userBooks, err := getUserBooks(); err == nil {
		if bytes, err := json.Marshal(userBooks); err == nil {
			books = string(bytes)
		}
	}

	return &pb.HelloReply{Message: "Hello " + in.Name + ", userId: " + userID + ", books: " + books}, nil
}

func (s *server) SayHelloAgain(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "Hello again " + in.Name}, nil
}

func (s *server) SayHelloStream(req *pb.HelloRequest, srv pb.Greeter_SayHelloStreamServer) error {
	srv.Send(&pb.HelloReply{Message: "Hello stream[1] " + req.GetName()})
	srv.Send(&pb.HelloReply{Message: "Hello stream[2] " + req.GetName()})
	return nil
}

func getUserInfo(ctx context.Context) UserInfo {
	resp, err := ctxhttp.Get(ctx, tracingClient, userServerURL+"/api/user/v1/userInfoBySession")

	if err != nil {
		apm.CaptureError(ctx, err).Send()
		log.Println("ERROR   request user info")
		return UserInfo{}
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Println("ERROR   reading user info")
		return UserInfo{}
	}

	var userInfo UserInfo
	json.Unmarshal(body, &userInfo)
	return userInfo
}

func init() {
	url := os.Getenv("USER_SERVER_URL")

	if url != "" {
		userServerURL = url
	}
}

func main() {
	handler := userHandler{}
	setUserCreatingHandler(&handler)
	initMQ()

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(apmgrpc.NewUnaryServerInterceptor()))
	pb.RegisterGreeterServer(s, &server{})
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
