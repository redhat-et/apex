# Jaywalking

Roads? Where we're going, we don't need roads - *Dr Emmett Brown*

### Controller Branch Quickstart

This branch runs a streaming service (in this case redis but could be any streamer), rest api and controller to provision mesh endpoints.

- Build for the node OS that is getting onboarded:

```
GOOS=linux GOARCH=amd64 go build -o jaywalk-amd64-linux
GOOS=darwin GOARCH=amd64 go build -o jaywalk-amd64-darwin
```
- Start redis instance in EC2 or somewhere all nodes can reach:

```
docker run \
    --name redis \
    -d -p 6379:6379 \
    redis redis-server \
    --requirepass <pass>
```

- Start the supervisor (this can be your laptop):

```
cd supervisor
go run supervisor.go \
    -streamer-address <REDIS_SERVER_ADDRESS> \
    -streamer-passwd <REDIS_PASSWD>
```

- Start the jaywalk agent:

```
sudo jaywalk --public-key=<NODE_WIREGUARD_PUB_KEY>  \
    --private-key=<NODE_WIREGUARD_PRIVATE_KEY>  \
    --controller=<REDIS_SERVER_ADDRESS> \
    --controller-password=<REDIS_PASSWD>
     --agent-mode
```

- Fire off the REST call to povision the nodes waiting for configuration
- Below is an example of provisioning 4 nodes. Three are publicly reachable in 
EC2 VPCs and one is a dev laptop that is behind a firewall with no ingress access.

```
curl --location --request POST 'http://localhost:8080/peers' \
--header 'Content-Type: application/json' \
--data-raw '[
    {
        "PublicKey": "oJlDE1y9xxmR6CIEYCSJAN+8b/RK73TpBYixlFiBJDM=",
        "EndpointIP": "192.168.1.40:51871",
        "AllowedIPs": "10.0.1.89/32"
    },
    {
        "AllowedIPs": "10.100.1.1/32",
        "EndpointIP": "3.94.59.204:51820",
        "PrivateKey": "WBydF4bEIs/uSR06hrsGa4vhgNxgR6rmR68CyOHMK18=",
        "PublicKey": "DUQ+TxqMya3YgRd1eXW/Tcg2+6wIX5uwEKqv6lOScAs="
    },
    {
        "AllowedIPs": "10.100.1.2/32",
        "EndpointIP": "18.205.149.74:51820",
        "PrivateKey": "2PHe/bV3Sya3sCamHWcnSQ8iQ4TIdPM4NGXGRlETIFA=",
        "PublicKey": "O3UVnLl6BFNYWf21tEDGpKbxYfzCp9LzwSXbtd9i+Eg="
    },
    {
        "AllowedIPs": "10.200.1.1/32",
        "EndpointIP": "34.224.78.66:51820",
        "PrivateKey": "4OXhMZdzodfOrmWvZyJRfiDEm+FJSwaEMI4co0XRP18=",
        "PublicKey": "M+BTP8LbMikKLufoTTI7tPL5Jf3SHhNki6SXEXa5Uic="
    }
]'
```

### Dev Quickstart

To deploy the current state run the following which deploys nodes across two VPCs and enables full mesh connectivity between them (simulating two data centers)

- Setup your aws profile with the required keys for ec2 provisioning:

```shell
vi ~/.aws/credentials
[default]
region = us-east-1
aws_access_key_id = <aws_access_key_id>
aws_secret_access_key = <aws_secret_access_key>
```

- Run the playbook (the jaywalk binary is stored in an S3 bucket and pulled down by ansible)

```shell
# Install Ansible if not already installed
python3 -m pip install --user ansible
ansible-playbook --version

# Run the playbook
git clone https://github.com/redhat-et/jaywalking.git
cd /jaywalking/ops/ansible/
ansible-playbook -vv ./deploy.yml 
```

- Once the nodes are finished provisioning, ssh to a node from the inventory and run the validation test. 

```shell
cat inventory.txt
ssh -i <key_name>.pem ubuntu@<ip_from_inventory>

./verify-connectivity.sh
node 10.200.1.6 is up
node 10.200.1.5 is up
node 10.200.1.4 is up
node 10.200.1.3 is up
node 10.200.1.2 is up
node 10.200.1.1 is up
node 10.100.1.6 is up
node 10.100.1.5 is up
node 10.100.1.4 is up
node 10.100.1.3 is up
node 10.100.1.2 is up
node 10.100.1.1 is up
```

- Add your own machine to the mesh, for example, a mac or linux dev machine by creating a toml file of any name with your host's details:

```toml
[Peers.mac]
PublicKey = "<Wireguard-PubKey>"
PrivateKey = "<Wireguard-PvtKey>"
EndpointIP = "192.168.1.6:51871" # Endpoint IP here is not nessecary but the code does not deal with an empty value there yet
AllowedIPs = "10.100.1.100/32"
```

- Copy the toml file into the `ops/ansible/peer-inventory` directory and re-run the playbook.

```shell
cp mac-node.toml ./peer-inventory

# rerun the playbook to update the nodes
ansible-playbook -vv ./deploy.yml 

# Build the binary for your OS (windows support available soon)
GOOS=linux GOARCH=amd64 go build -o jaywalk-amd64-linux
GOOS=darwin GOARCH=amd64 go build -o jaywalk-amd64-darwin

# On your dev machine, then run:
sudo ./jaywalk-amd64-darwin --public-key=<wireguard_public_key>  --config=ops/ansible/endpoints.toml
```

- Now your host should be able to reach all nodes in the mesh, verify with `./verify-connectivity.sh` located in the ansible directory.

- Tear down the environment with:
```
ansible-playbook terminate-instances.yml
```