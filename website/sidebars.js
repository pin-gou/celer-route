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
        'features/dashboard-auth',
        'features/provider-cooldown',
        'features/rtk',
      ],
    },
    {
      type: 'category',
      label: 'Provider 接入',
      items: ['providers/supported-providers'],
    },
  ],
};