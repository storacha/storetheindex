resource "aws_ecs_service" "ingest" {
  name                 = "${var.environment}-${var.app}-service"
  cluster              = var.ecs_cluster.id
  task_definition      = aws_ecs_task_definition.ingest.arn
  desired_count        = 1
  force_new_deployment = true
  load_balancer {
    target_group_arn = aws_lb_target_group.blue.arn
    container_name   = "ingest"
    container_port   =  local.config.httpport # Application Port
  }
  launch_type = "FARGATE"
  network_configuration {
    security_groups  = [aws_security_group.container_sg.id]
    subnets          = var.vpc.subnet_ids.private
    assign_public_ip = false
  }
  deployment_controller {
    type = "CODE_DEPLOY"
  }
  lifecycle {
    ignore_changes = [load_balancer, task_definition, desired_count]
  }
}

resource "aws_security_group" "container_sg" {
  name        = "${var.environment}-${var.app}-container-sg"
  description = "allow inbound traffic to the containers"
  vpc_id      = var.vpc.id
  tags = {
    "Name" = "${var.environment}-${var.app}-container-sg"
  }

  egress {
    from_port   = 0
    to_port     = 65535
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    security_groups = [var.lb_security_group.id]
    from_port       = local.config.httpport
    to_port         = local.config.httpport
    protocol        = "tcp"
  }
}
