-- Cập nhật bootstrap path sau Phase B (core vs addons)

UPDATE platform_plugins SET bootstrap = 'addons/install-rancher.sh' WHERE name = 'rancher';
UPDATE platform_plugins SET bootstrap = 'addons/install-harbor.sh' WHERE name = 'harbor';
