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
        'features/rtk',
      ],
    },
    {
      type: 'category',
      label: 'AI 客户端接入',
      items: ['clients/agent-setup'],
    },
    {
      type: 'category',
      label: '提供商接入',
      items: ['providers/supported-providers', 'providers/recommended-providers', 'providers/provider-detail'],
    },
  ],
};