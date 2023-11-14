# install Go
curl -OL https://golang.org/dl/go1.21.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xvf go1.21.4.linux-amd64.tar.gz

echo "export PATH=\$PATH:/usr/local/go/bin" >> ~/.bashrc
. ~/.bashrc

go version

rm go1.21.4.linux-amd64.tar.gz

# build code
go build -o lockserver
