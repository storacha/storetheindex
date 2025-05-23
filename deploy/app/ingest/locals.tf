locals {
    config = var.deployment_config != null ? var.deployment_config : var.environment == "prod" ? {
    cpu = 1024
    memory = 2048
    httpport = var.httpport
    readonly = !var.write_to_container
  } : {
    cpu = 256
    memory = 512
    httpport = var.httpport
    readonly = !var.write_to_container
  }
}