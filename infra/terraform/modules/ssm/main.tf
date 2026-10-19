variable "prefijo" { type = string }
variable "parametros" { type = list(string) }

# Los secretos se crean VACÍOS y se cargan con la CLI. Si el valor se pusiera en Terraform
# quedaría en el tfstate, que es texto plano: sería mover el secreto de lugar, no protegerlo.
resource "aws_ssm_parameter" "this" {
  for_each = toset(var.parametros)

  name  = "/${var.prefijo}/${each.value}"
  type  = "SecureString"
  value = "sin-definir"

  lifecycle {
    ignore_changes = [value]
  }
}

output "prefijo" { value = "/${var.prefijo}" }
output "prefijo_arn" { value = "arn:aws:ssm:*:*:parameter/${var.prefijo}/*" }
