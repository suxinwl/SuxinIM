## Environment Prepare

Install `node_modules`:

```bash
npm install
```

or

```bash
yarn
```

## Change your config

> src->config
>
> 如果以 ip+端口形式配置 建议**不要修改端口号** 按照下方对应关系进行配置

```bash
export const API_URL = 'http://43.154.157.177:10002';
export const CHAT_URL = 'http://43.154.157.177:10008';
export const ACCOUNT_URL = 'http://43.154.157.177:10009';
```

### Start project

```bash
npm start
```

### Build project

> generate resource folder `dist`

```bash
npm run build
```

### Check code style

```bash
npm run lint
```

You can also use script to auto fix some lint error:

```bash
npm run lint:fix
```
