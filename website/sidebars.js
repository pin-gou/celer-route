export default {
  docs: [
    'intro',
    {
      type: 'category',
      label: '入门',
      collapsed: false,
      items: [
        'deployment/sqlite',
        'deployment/postgres',
        'features/data-storage',
      ],
    },
    {
      type: 'category',
      label: 'pg-skills',
      collapsed: false,
      items: [
        'tutorials/celer-route',
        {
          type: 'category',
          label: '相关参考',
          items: [
            'pg-skills/configuration',
            'pg-skills/project-structure',
            'pg-skills/existing-projects',
            'pg-skills/how-commands-work',
            'pg-skills/model-routing',
            'pg-skills/tutorials/opencode',
            'pg-skills/troubleshooting',
          ],
        },
      ],
    },
    {
      type: 'category',
      label: '功能',
      items: [
        'features/dashboard-auth',
        'features/i18n',
        'features/provider-cooldown',
      ],
    },
    {
      type: 'category',
      label: 'Provider 接入',
      items: [
        'providers/supported-providers/coze',
        'providers/supported-providers/coze_cn',
        'providers/supported-providers/baichuan',
        'providers/supported-providers/modelscope',
        'providers/supported-providers/stepfun',
        'providers/supported-providers/xiaomi_mimo',
        'providers/supported-providers/iflytek',
        'providers/supported-providers/gmicloud',
      ],
    },
    {
      type: 'category',
      label: '技术参考',
      items: ['reference/cooldown-logging'],
    },
  ],
};
