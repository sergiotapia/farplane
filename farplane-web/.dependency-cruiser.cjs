'use strict'
/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: 'no-circular',
      severity: 'error',
      comment: 'Circular dependencies make modules harder to change.',
      from: {},
      to: { circular: true },
    },
    {
      name: 'no-orphans',
      severity: 'error',
      comment: 'Orphan modules are dead weight; delete or wire them.',
      from: {
        orphan: true,
        pathNot: [
          '^src/main\\.tsx$',
          '^src/router\\.tsx$',
          '^src/test/',
          '^src/global\\.d\\.ts$',
          '\\.test\\.(ts|tsx)$',
          '\\.spec\\.(ts|tsx)$',
        ],
      },
      to: {},
    },
    {
      name: 'lib-not-to-ui',
      severity: 'error',
      comment: 'lib must stay free of React UI imports.',
      from: { path: '^src/lib' },
      to: { path: '^src/components' },
    },
    {
      name: 'lib-not-to-routes',
      severity: 'error',
      comment: 'lib must not import routes.',
      from: { path: '^src/lib' },
      to: { path: '^src/routes' },
    },
    {
      name: 'lib-not-to-hooks',
      severity: 'error',
      comment: 'lib must not import React hooks modules.',
      from: { path: '^src/lib' },
      to: { path: '^src/hooks' },
    },
    {
      name: 'ui-not-to-routes',
      severity: 'error',
      comment: 'UI primitives must not import route modules.',
      from: { path: '^src/components/ui' },
      to: { path: '^src/routes' },
    },
    {
      name: 'ui-not-to-lib-api',
      severity: 'error',
      comment: 'UI primitives must not call the Farplane API client.',
      from: { path: '^src/components/ui' },
      to: { path: '^src/lib/api(\\.ts)?$' },
    },
    {
      name: 'hooks-not-to-routes',
      severity: 'error',
      comment: 'Hooks must not import route modules.',
      from: { path: '^src/hooks' },
      to: { path: '^src/routes' },
    },
    {
      name: 'no-non-package-json',
      severity: 'error',
      comment: 'Do not import deps that are missing from package.json.',
      from: {},
      to: { dependencyTypes: ['npm-no-pkg', 'npm-unknown'] },
    },
    {
      name: 'not-to-dev-dep',
      severity: 'error',
      comment: 'Production code must not import devDependencies.',
      from: {
        pathNot: [
          '\\.test\\.(ts|tsx)$',
          '\\.spec\\.(ts|tsx)$',
          '^src/test/',
          '^e2e/',
          '\\.config\\.(ts|js|cjs|mjs)$',
        ],
      },
      to: { dependencyTypes: ['npm-dev'] },
    },
  ],
  options: {
    doNotFollow: {
      path: ['node_modules', 'dist', 'coverage', '\\.features-gen'],
    },
    tsPreCompilationDeps: true,
    tsConfig: { fileName: 'tsconfig.json' },
    enhancedResolveOptions: {
      exportsFields: ['exports'],
      conditionNames: ['import', 'require', 'node', 'default'],
      mainFields: ['module', 'main', 'types', 'typings'],
    },
    reporterOptions: {
      text: { highlightFocused: true },
    },
  },
}
