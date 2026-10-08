# Administration interface

The theme, navigation proportions, card treatments, and interaction styling in
this directory are adapted from [Remnawave frontend](https://github.com/remnawave/frontend),
version 3.4.5, copyright Remnawave contributors. The corresponding license is
preserved in `LICENSE-Remnawave`. Modified frontend sources are provided here.

The administration interface uses React, Mantine, and
`@kastov/mantine-react-table-open`, the same UI libraries used by Remnawave.
These libraries retain their own licenses in their published packages.

Run `npm ci` and `npm run build` in this directory after changing the sources.
The checked-in JavaScript and CSS bundles in `internal/miniapp/static` are served
by the Go application's embedded filesystem. Styles are scoped to the admin
surface so customer cabinet preferences cannot change the admin theme.
