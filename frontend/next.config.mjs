/** @type {import('next').NextConfig} */
const nextConfig = {
  // Export estático: el panel es HTML+JS que le habla a la API desde el navegador. Sin SSR,
  // porque no hay SEO que ganar en un panel privado y un sitio estático se sirve desde
  // S3+CloudFront por centavos. La costura con el backend es una sola: NEXT_PUBLIC_API_URL.
  output: 'export',
  trailingSlash: true,
  images: { unoptimized: true },
}
export default nextConfig
