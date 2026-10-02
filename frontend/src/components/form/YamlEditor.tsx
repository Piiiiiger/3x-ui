import { forwardRef } from 'react';
import { useTranslation } from 'react-i18next';

import CodeEditor, { type CodeEditorHandle, type CodeEditorProps } from './CodeEditor';

export type YamlEditorProps = CodeEditorProps;
export type YamlEditorHandle = CodeEditorHandle;

const YamlEditor = forwardRef<YamlEditorHandle, YamlEditorProps>(function YamlEditor(props, ref) {
  const { t } = useTranslation();
  return <CodeEditor ref={ref} {...props} language="yaml" label={t('yamlEditor')} />;
});

export default YamlEditor;
