variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "aws_profile" {
  type        = string
  default     = null
  description = "Perfil de la CLI de AWS. null usa el de por defecto."
}

variable "proyecto" {
  type    = string
  default = "logs-consumer"
}

variable "pipeline" {
  type        = string
  default     = "panel"
  description = "Segundo nivel de la etiqueta. Sirve para filtrar el presupuesto."
}

# Los nombres de bucket son únicos en TODO AWS, así que necesitan un sufijo propio. En vez de
# pedirte que escribas tu id de cuenta en un archivo, se deriva de las credenciales con las que
# corras Terraform: así el repositorio no contiene nada que identifique la cuenta.
variable "bucket_datos" {
  type        = string
  default     = ""
  description = "Bucket de sobres, resúmenes y estado. Vacío = <proyecto>-datos-<id de cuenta>."
}

variable "bucket_web" {
  type        = string
  default     = ""
  description = "Bucket del panel estático. Vacío = <proyecto>-panel-<id de cuenta>."
}

variable "tag_imagen" {
  type    = string
  default = "latest"
}

variable "memoria_mb" {
  type        = number
  default     = 512
  description = "En Lambda la CPU escala con la memoria. 512 alcanza y sobra para listar objetos."
}

variable "timeout_s" {
  type        = number
  default     = 30
  description = "Un diagnóstico con el modelo puede tardar; el resto responde en milisegundos."
}

variable "dias_retencion_logs" {
  type    = number
  default = 30
}

variable "dias_retencion_sobres" {
  type        = number
  default     = 0
  description = "0 = no expiran. Un año de corridas de cinco pipelines son unos pocos MB."
}

variable "imagenes_a_retener" {
  type    = number
  default = 5
}

variable "dominio_panel" {
  type        = string
  default     = ""
  description = "Dominio propio del panel. Vacío usa el de CloudFront."
}

variable "certificado_arn" {
  type        = string
  default     = ""
  description = "Certificado ACM en us-east-1. Obligatorio si hay dominio_panel."
}

variable "diagnostico" {
  type        = string
  default     = "off"
  description = "on enciende la llamada al modelo. Requiere la llave cargada en SSM."
}

variable "modelo" {
  type    = string
  default = "claude-opus-5"
}

variable "presupuesto_usd" {
  type        = string
  default     = "5"
  description = "Lo esperado es menos de USD 1/mes; 5 deja margen para que la alarma signifique algo."
}

variable "email_alertas" {
  type        = string
  description = "Destinatario de las alertas de presupuesto."
}
