import { useRouteError } from 'react-router';
import { Button, Result } from 'antd';

export default function RouteLoadError() {
  const error = useRouteError();
  const detail = error instanceof Error ? error.message : '';
  const missingAsset = /dynamically imported module|Loading chunk|module script|CSS chunk/i.test(
    detail,
  );
  return (
    <Result
      status="warning"
      title={missingAsset ? '页面资源未能加载' : '页面暂时无法显示'}
      subTitle={
        missingAsset
          ? '页面可能已更新，或网络暂时中断。刷新后再打开即可；未保存的输入会丢失。'
          : '请刷新页面重试。如果仍然出现，请联系管理员。'
      }
      extra={
        <Button type="primary" onClick={() => window.location.reload()}>
          刷新并恢复页面
        </Button>
      }
    />
  );
}
