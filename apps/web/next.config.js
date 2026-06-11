/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  experimental: {
    serverComponentsExternalPackages: [],
  },
  // fix: ensure docs routes are included in standalone build
  poweredByHeader: false,
}

module.exports = nextConfig
