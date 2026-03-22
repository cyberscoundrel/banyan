const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');
const webpack = require('webpack');

module.exports = (env, argv) => {
  const isProduction = argv.mode === 'production';
  
  return {
    entry: './src/index.tsx',
    output: {
      path: path.resolve(__dirname, 'dist'),
      filename: 'bundle.[contenthash].js',
      clean: true,
      publicPath: '/',
    },
    resolve: {
      extensions: ['.tsx', '.ts', '.js', '.jsx'],
    },
    module: {
      rules: [
        {
          test: /\.tsx?$/,
          use: 'ts-loader',
          exclude: /node_modules/,
        },
        {
          test: /\.css$/i,
          use: ['style-loader', 'css-loader', 'postcss-loader'],
        },
        {
          test: /\.md$/,
          type: 'asset/source',
        },
      ],
    },
    plugins: [
      new HtmlWebpackPlugin({
        template: './public/index.html',
        favicon: './public/fig.svg',
      }),
      new webpack.DefinePlugin({
        // Use isProduction to set NODE_ENV - in dev mode, downloads come from local filesystem
        // via devServer static config; in production (Docker), downloads come from Cloudflare bucket
        'process.env.NODE_ENV': JSON.stringify(isProduction ? 'production' : 'development'),
      }),
    ],
    devServer: {
      static: [
        {
          directory: path.join(__dirname, 'public'),
        },
        {
          // Serve deliverables from monorepo dist in development
          directory: path.resolve(__dirname, '../../dist/deliverables'),
          publicPath: '/releases',
        },
      ],
      compress: true,
      port: 3000,
      hot: true,
      historyApiFallback: true,
      headers: {
        // Allow downloads
        'Access-Control-Allow-Origin': '*',
      },
    },
  };
};
