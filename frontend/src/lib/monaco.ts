import { loader } from '@monaco-editor/react';
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api';
import 'monaco-editor/esm/vs/editor/editor.all.js';
import 'monaco-editor/esm/vs/basic-languages/yaml/yaml.contribution';
import editorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import { configureMonacoYaml } from 'monaco-yaml';
import yamlWorker from 'monaco-yaml/yaml.worker?worker';

import { setSchemaApplier } from './schema';

declare global {
  interface Window {
    MonacoEnvironment?: monaco.Environment;
  }
}

window.MonacoEnvironment = {
  getWorker(_workerId: string, label: string) {
    if (label === 'yaml') {
      return new yamlWorker();
    }
    return new editorWorker();
  },
};

const monacoYaml = configureMonacoYaml(monaco, {
  enableSchemaRequest: false,
  validate: true,
  hover: true,
  completion: true,
  schemas: [],
});

setSchemaApplier((schemas) => {
  void monacoYaml.update({ schemas });
});

loader.config({ monaco });
