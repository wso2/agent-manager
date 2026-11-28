path "secret/data/amp/*" {
  capabilities = ["create", "read"]
}
path "secret/destroy/amp/*" {
  capabilities = ["update"]
}
path "secret/metadata/amp/*" {
  capabilities = ["delete"]
}
