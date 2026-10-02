import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react';
import { EditorView, basicSetup } from 'codemirror';
import { EditorState, Compartment } from '@codemirror/state';
import { json, jsonParseLinter } from '@codemirror/lang-json';
import { yaml } from '@codemirror/lang-yaml';
import { lintGutter, linter } from '@codemirror/lint';
import { oneDarkHighlightStyle } from '@codemirror/theme-one-dark';
import { syntaxHighlighting } from '@codemirror/language';
import { keymap } from '@codemirror/view';
import { indentWithTab } from '@codemirror/commands';

import { useTheme } from '@/hooks/useTheme';
import './CodeEditor.css';

export interface CodeEditorProps {
  value: string;
  onChange?: (next: string) => void;
  minHeight?: string;
  maxHeight?: string;
  readOnly?: boolean;
}

export interface CodeEditorHandle {
  focus: () => void;
}

export type CodeEditorLanguage = 'json' | 'yaml';

// JSON gets a parse linter; YAML is checked by the server on save.
function languageExtensions(language: CodeEditorLanguage) {
  return language === 'json' ? [json(), linter(jsonParseLinter()), lintGutter()] : [yaml()];
}

interface DarkPalette {
  bg: string;
  panelBg: string;
  activeBg: string;
  border: string;
  selection: string;
}

function buildDarkTheme({ bg, panelBg, activeBg, border, selection }: DarkPalette) {
  return EditorView.theme(
    {
      '&': { color: '#dcdcdc', backgroundColor: bg },
      '.cm-content': { caretColor: '#dcdcdc' },
      '.cm-cursor, .cm-dropCursor': { borderLeftColor: '#dcdcdc' },
      '.cm-gutters': {
        backgroundColor: bg,
        borderRight: `1px solid ${border}`,
        color: '#6a6a6a',
      },
      '.cm-activeLine': { backgroundColor: activeBg },
      '.cm-activeLineGutter': { backgroundColor: activeBg, color: '#dcdcdc' },
      '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
        backgroundColor: selection,
      },
      '.cm-panels': { backgroundColor: panelBg, color: '#dcdcdc' },
      '.cm-panels.cm-panels-top': { borderBottom: `1px solid ${border}` },
      '.cm-panels.cm-panels-bottom': { borderTop: `1px solid ${border}` },
      '.cm-tooltip': {
        backgroundColor: panelBg,
        border: `1px solid ${border}`,
        color: '#dcdcdc',
      },
    },
    { dark: true },
  );
}

const darkTheme = buildDarkTheme({
  bg: '#1e1e1e',
  panelBg: '#2d2d30',
  activeBg: '#252526',
  border: '#3a3a3c',
  selection: '#3a3a3c',
});

const ultraDarkTheme = buildDarkTheme({
  bg: '#0a0a0a',
  panelBg: '#141414',
  activeBg: '#141414',
  border: '#1f1f1f',
  selection: '#2a2a2a',
});

function themeExtension(isDark: boolean, isUltra: boolean) {
  if (!isDark) return [];
  const chrome = isUltra ? ultraDarkTheme : darkTheme;
  return [chrome, syntaxHighlighting(oneDarkHighlightStyle)];
}

const CodeEditor = forwardRef<
  CodeEditorHandle,
  CodeEditorProps & { language: CodeEditorLanguage; label: string }
>(function CodeEditor(
  { value, onChange, minHeight = '320px', maxHeight = '600px', readOnly = false, language, label },
  ref,
) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const viewRef = useRef<EditorView | null>(null);
  const themeCompartmentRef = useRef<Compartment>(new Compartment());
  const readonlyCompartmentRef = useRef<Compartment>(new Compartment());
  const onChangeRef = useRef(onChange);
  const valueRef = useRef(value);
  const { isDark, isUltra } = useTheme();

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);

  useImperativeHandle(ref, () => ({
    focus: () => viewRef.current?.focus(),
  }));

  useEffect(() => {
    if (!hostRef.current) return;

    const updateListener = EditorView.updateListener.of((u) => {
      if (!u.docChanged) return;
      const next = u.state.doc.toString();
      if (next === valueRef.current) return;
      valueRef.current = next;
      onChangeRef.current?.(next);
    });

    const view = new EditorView({
      parent: hostRef.current,
      state: EditorState.create({
        doc: value,
        extensions: [
          basicSetup,
          EditorView.contentAttributes.of({ 'aria-label': label }),
          keymap.of([indentWithTab]),
          ...languageExtensions(language),
          EditorView.lineWrapping,
          updateListener,
          themeCompartmentRef.current.of(themeExtension(isDark, isUltra)),
          readonlyCompartmentRef.current.of(EditorState.readOnly.of(readOnly)),
          EditorView.theme({
            '&': { height: '100%' },
            '.cm-scroller': {
              fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
              fontSize: '12px',
              minHeight,
              maxHeight,
            },
          }),
        ],
      }),
    });

    viewRef.current = view;

    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const current = view.state.doc.toString();
    if (value === current) return;
    valueRef.current = value;
    view.dispatch({ changes: { from: 0, to: current.length, insert: value } });
  }, [value]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    view.dispatch({
      effects: themeCompartmentRef.current.reconfigure(themeExtension(isDark, isUltra)),
    });
  }, [isDark, isUltra]);

  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    view.dispatch({
      effects: readonlyCompartmentRef.current.reconfigure(EditorState.readOnly.of(readOnly)),
    });
  }, [readOnly]);

  return <div ref={hostRef} className="code-editor-host" aria-label={label} />;
});

export default CodeEditor;
