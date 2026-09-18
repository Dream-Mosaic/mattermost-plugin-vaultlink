const path = require('path');

module.exports = {
    entry: './src/index.tsx',
    resolve: {
        extensions: ['.ts', '.tsx', '.js'],
        modules: ['src', 'node_modules'],
        alias: {
            manifest: path.resolve(__dirname, 'src/manifest'),
        },
    },
    module: {
        rules: [
            {
                test: /\.tsx?$/,
                use: {
                    loader: 'ts-loader',
                    options: {transpileOnly: true},
                },
                exclude: /node_modules/,
            },
        ],
    },
    externalsType: 'var',
    externals: {
        react: 'React',
        'react-dom': 'ReactDOM',
    },
    output: {
        devtoolNamespace: 'vaultlink',
        path: path.join(__dirname, '/dist'),
        publicPath: '/',
        filename: 'main.js',

        // No libraryTarget: Mattermost loads plugin bundles as plain browser
        // <script> tags, so the output must be a self-contained IIFE. Setting
        // libraryTarget: 'commonjs2' wraps the bundle in `module.exports = ...`,
        // which throws "module is not defined" in the browser. Externals are
        // resolved as globals via externalsType: 'var' (window.React, etc.).
    },
};
