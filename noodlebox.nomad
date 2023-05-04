job "NoodleBox" {
  datacenters = ["dc1"]

  group "noodlebox" {
    network {
      port "keydb" {to=6379}
      port "http" {static=8080}
    }

    task "keydb" {

      driver = "docker"
      config {
        image = "eqalpha/keydb:alpine"
        ports = ["keydb"]
      }
    }
  }
}