# install Go
curl -OL https://golang.org/dl/go1.21.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xvf go1.21.4.linux-amd64.tar.gz
rm go1.21.4.linux-amd64.tar.gz

export PATH=\$PATH:/usr/local/go/bin
hash -r

go version

# build code
go build -o client_benchmark
