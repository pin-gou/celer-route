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
      label: 'pg-skills 实践',
      items: ['tutorials/celer-route'],
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
