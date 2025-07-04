resource "aws_lb_target_group" "blue" {
  name        = "${var.environment}-${var.app}-blue"
  port        = local.config.httpport
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = var.vpc.id
  health_check {
    path    = "/health"
    matcher = "200,301,302,404"
  }
}

resource "aws_lb_target_group" "green" {
  name        = "${var.environment}-${var.app}-green"
  port        = local.config.httpport
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = var.vpc.id
  health_check {
    path    = "/health"
    matcher = "200,301,302,404"
  }
}

resource "aws_lb_listener_rule" "announce" {
  listener_arn = var.lb_listener.arn
  priority     = 10

  condition {
    path_pattern {
      values = ["/announce*"]
    }
  }

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.blue.arn
  }

  lifecycle {
    ignore_changes = [action]
  }

  tags = {
    Name = "announce-to-ingest-service"
  }
}
