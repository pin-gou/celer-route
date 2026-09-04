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
      label: '功能',
      items: [
        'features/routing',
        'features/routing-example',
        'features/dashboard-auth',
        'features/provider-cooldown',
        'features/agent-setup',
        'features/rtk',
      ],
    },
    {
      type: 'category',
      label: 'Provider 接入',
      items: ['providers/supported-providers', 'providers/recommended-providers'],
    },
  ],
};