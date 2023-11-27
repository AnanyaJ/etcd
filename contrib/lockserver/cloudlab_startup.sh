# install Go
curl -OL https://golang.org/dl/go1.21.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xvf go1.21.4.linux-amd64.tar.gz
rm go1.21.4.linux-amd64.tar.gz

export PATH=$PATH:/usr/local/go/bin
hash -r

go version

# build code
go get go.etcd.io/etcd/v3/contrib/lockserver
go build -o lockserver

export CLUSTER="http://10.10.1.2:12379,http://10.10.1.3:12379,http://10.10.1.4:12379"