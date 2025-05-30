data "aws_region" "current" {}

# fetch the task definition for the find service to inherit its config
data "aws_ecs_task_definition" "find" {
  # this weird conditional is a workaround to avoid terraform failing when the find task doesn't exist (either it
  # hasn't been deployed yet or it was deleted). The reference to the revision makes this data block dependent on
  # the find task existing. See https://github.com/hashicorp/terraform-provider-aws/pull/10247
  task_definition = var.find_task.revision ? var.find_task.family : var.find_task.family
}

locals {
  execution_role_arn = data.aws_ecs_task_definition.find.execution_role_arn
  task_role_arn      = data.aws_ecs_task_definition.find.task_role_arn
}

resource "aws_ecs_task_definition" "ingest" {
  family                   = "${var.environment}-${var.app}"
  execution_role_arn       = local.execution_role_arn
  task_role_arn            = local.task_role_arn
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = local.config.cpu
  memory                   = local.config.memory
  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  container_definitions = jsonencode([
    {
      name                   = "ingest"
      image                  = var.image_tag
      cpu                    = local.config.cpu
      memory                 = local.config.memory
      essential              = true
      readonlyRootFilesystem = local.config.readonly
      portMappings = [
        {
          containerPort = local.config.httpport
          hostPort      = local.config.httpport
          protocol      = "tcp"
          appProtocol   = "http"
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = var.ecs_log_group.name
          awslogs-region        = data.aws_region.current.name
          awslogs-stream-prefix = "ecs"
        }
      }
      healthCheck = var.healthcheck ? {
        command     = ["CMD-SHELL", "curl -f http://localhost:${local.config.httpport}/health >> /proc/1/fd/1 2>&1 || exit 1"]
        interval    = 30
        retries     = 3
        timeout     = 5
        startPeriod = 10
      } : null
      environment = concat(var.env_vars,
        [ for key, bucket in var.buckets : {
          name = "${upper(key)}_BUCKET_NAME"
          value = bucket.bucket
        }],
        [ for key, table in var.tables : {
          name = "${upper(key)}_TABLE_ID"
          value = table.id
        }]
      ),
      environmentFiles = [ for env_file in aws_s3_object.env_files : { "value": env_file.arn, "type": "s3" }]
      secrets = var.secrets
    }
  ])
}

resource "aws_s3_object" "env_files" {
  for_each = { for env_file in var.env_files : basename(env_file) => env_file}
  bucket = var.env_files_bucket_id
  key = "${var.environment}-${var.app}-${each.key}.env"
  source = each.value
  etag = filemd5(each.value)
}
