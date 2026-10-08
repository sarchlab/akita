// @ts-check
// The runner and its subpackages share one documentation tree. Keep the
// established public package URLs and ordering.
/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  coreGroupSidebar: [
    {type: 'doc', id: 'hooking/README', label: 'hooking'},
    {type: 'doc', id: 'naming/README', label: 'naming'},
    {type: 'doc', id: 'timing/README', label: 'timing'},
    {type: 'doc', id: 'queueing/README', label: 'queueing'},
    {type: 'doc', id: 'datarecording/README', label: 'datarecording'},
    {type: 'category', label: 'messaging', items: [
      {type: 'doc', id: 'messaging/README', label: 'Overview'},
      {type: 'doc', id: 'messaging/twowaybuffered/README', label: 'twowaybuffered'},
      {type: 'doc', id: 'messaging/direct/README', label: 'direct'},
    ]},
    {type: 'doc', id: 'modeling/README', label: 'modeling'},
    {type: 'doc', id: 'tracing/README', label: 'tracing'},
    {type: 'doc', id: 'README', label: 'sim'},
    {type: 'link', label: 'examples', href: '/packages/examples/'},
  ],
};
export default sidebars;
