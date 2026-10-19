variable "prefijo" { type = string }
variable "bucket_web_id" { type = string }
variable "bucket_web_dominio" { type = string }
variable "lambda_url" { type = string }
variable "dominio" { type = string }
variable "certificado_arn" { type = string }

resource "aws_cloudfront_origin_access_control" "web" {
  name                              = "${var.prefijo}-web"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

locals {
  # La Function URL viene como https://xxx.lambda-url.region.on.aws/ y CloudFront quiere el
  # host pelado.
  lambda_host = replace(replace(var.lambda_url, "https://", ""), "/", "")
}

# CloudFront con origen S3 por OAC NO resuelve indices de directorio: `/salud/` busca la clave
# `salud/`, que no existe —el objeto es `salud/index.html`— y S3 devuelve 403. `default_root_object`
# solo cubre la raiz. Sin esta funcion, toda la navegacion del panel da 403 salvo la portada.
resource "aws_cloudfront_function" "rutas" {
  name    = "${var.prefijo}-rutas"
  runtime = "cloudfront-js-2.0"
  comment = "Resuelve index.html para las rutas del export estatico"
  publish = true
  code    = <<-JS
    function handler(event) {
      var request = event.request;
      var uri = request.uri;
      if (uri.endsWith('/')) {
        request.uri = uri + 'index.html';
      } else if (!uri.includes('.')) {
        request.uri = uri + '/index.html';
      }
      return request;
    }
  JS
}

resource "aws_cloudfront_distribution" "this" {
  enabled             = true
  comment             = var.prefijo
  default_root_object = "index.html"
  price_class         = "PriceClass_100" # Norteamérica y Europa: es un panel privado, no un sitio global
  aliases             = var.dominio != "" ? [var.dominio] : []

  origin {
    origin_id                = "web"
    domain_name              = var.bucket_web_dominio
    origin_access_control_id = aws_cloudfront_origin_access_control.web.id
  }

  origin {
    origin_id   = "api"
    domain_name = local.lambda_host
    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  # El panel: estático y cacheable.
  default_cache_behavior {
    target_origin_id       = "web"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD", "OPTIONS"]
    cached_methods         = ["GET", "HEAD"]
    cache_policy_id        = data.aws_cloudfront_cache_policy.optimizada.id

    function_association {
      event_type   = "viewer-request"
      function_arn = aws_cloudfront_function.rutas.arn
    }
  }

  # La API: un solo origen para todo evita CORS en producción y deja el token en el mismo
  # dominio. Nada se cachea —un panel que muestra corridas viejas es peor que uno lento— y hay
  # que reenviar el Authorization, que la política por defecto descarta.
  ordered_cache_behavior {
    path_pattern             = "/api/*"
    target_origin_id         = "api"
    viewer_protocol_policy   = "https-only"
    allowed_methods          = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods           = ["GET", "HEAD"]
    cache_policy_id          = data.aws_cloudfront_cache_policy.sin_cache.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.todo_menos_host.id
  }

  # Next con `output: export` y trailingSlash genera un 404.html propio.
  custom_error_response {
    error_code         = 404
    response_code      = 404
    response_page_path = "/404.html"
  }

  restrictions {
    geo_restriction { restriction_type = "none" }
  }

  viewer_certificate {
    cloudfront_default_certificate = var.dominio == ""
    acm_certificate_arn            = var.dominio != "" ? var.certificado_arn : null
    ssl_support_method             = var.dominio != "" ? "sni-only" : null
    minimum_protocol_version       = var.dominio != "" ? "TLSv1.2_2021" : null
  }
}

data "aws_cloudfront_cache_policy" "optimizada" { name = "Managed-CachingOptimized" }
data "aws_cloudfront_cache_policy" "sin_cache" { name = "Managed-CachingDisabled" }
data "aws_cloudfront_origin_request_policy" "todo_menos_host" { name = "Managed-AllViewerExceptHostHeader" }

output "dominio" { value = "https://${aws_cloudfront_distribution.this.domain_name}" }
output "arn" { value = aws_cloudfront_distribution.this.arn }
