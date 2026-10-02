import { forwardRef } from 'react';
import { useTranslation } from 'react-i18next';

import CodeEditor, { type CodeEditorHandle, type CodeEditorProps } from './CodeEditor';

export type JsonEditorProps = CodeEditorProps;
export type JsonEditorHandle = CodeEditorHandle;

const JsonEditor = forwardRef<JsonEditorHandle, JsonEditorProps>(function JsonEditor(props, ref) {
  const { t } = useTranslation();
  return <CodeEditor ref={ref} {...props} language="json" label={t('jsonEditor')} />;
});

export default JsonEditor;
