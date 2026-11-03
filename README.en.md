[简体中文](README.md) | [English](README.en.md) | [日本語](README.ja.md)

# Hitbox Map

A local button mapper for GP2040-CE controllers. Arrange the buttons to match your controller, identify each physical input once, then click its position to remap it.

![English remapping interface with an example layout](docs/images/remap-en.jpg)

The screenshots show the English interface with a simulated device and an example layout.

## Start and connect

1. Download the Windows x64 ZIP from [Releases](https://github.com/MooWantFree/GP2040-hitbox-easy-UI/releases) and extract it to a writable folder. If no release is available, follow the [build instructions](docs/releasing.md) to get a build from GitHub Actions.
2. Double-click `HitboxMap.exe`. The management page opens in your browser and an icon appears in the system tray. No additional runtime or firmware flash is needed.
3. Hold Menu / Start while plugging in your controller to enter Web Config mode. If you changed the startup shortcut, use that shortcut instead.
4. Enter the device address, normally `192.168.7.1`, and click **Connect**.

The management page starts at `http://127.0.0.1:29301` by default. Launching the app again opens the existing instance. If another app occupies the port, Hitbox Map tries the next one.

## Arrange the buttons

Select **Layout** and drag a small circle, circle, capsule or joystick onto the canvas. Drag existing buttons to move them. Select a button to rename, resize or delete it. Drag the joystick by its central handle to move the whole group.

![English layout editor with four draggable shapes](docs/images/layout-en.jpg)

For keyboard controls, use Tab to focus a shape and press Enter or Space to add it at the center. Move it with the arrow keys; hold Shift for larger steps.

## Identify physical inputs

1. Select **Identify**, then click a position on the canvas.
2. Release all physical buttons, then press and release the button that belongs at that position. Each attempt waits up to five seconds.
3. Repeat for the remaining buttons. Identify each joystick direction separately, without pushing diagonally.

If an input already belongs to another position, you can confirm **Replace binding**. The old position becomes unidentified. Two physical buttons wired to the same pin cannot be remapped separately. Identify the inputs again after changing controllers or wiring.

Button positions, shapes, names and input bindings save automatically. Once the page shows **Layout and bindings saved**, they will be available the next time you open it.

## Remap and save

1. Select **Remap**. Under **Button labels**, choose GP2040, Xbox, PlayStation, Switch or Arcade.
2. Click a button on the canvas, then choose its output in the right panel.
3. Click **Save to device**, review the changes and click **Confirm save**.
4. Open **More → Firmware settings** and reboot the device into controller mode.

Button labels change the displayed names only. Use Firmware settings to change the input mode, SOCD, macros or lighting. Keep a Menu / Start output available so you can enter Web Config mode again.

| Control | How to use it |
| --- | --- |
| Swap outputs | Select a button, click Swap outputs, then select another identified position. |
| Combination | Enable it and select several outputs, such as X + LT, to send them together. |
| Edit profile | Choose the profile to edit. Saving writes only that profile. Use Firmware settings to change the active profile. |
| Copy profile | Choose a source and copy it to the current draft. Unsaved source changes are included. |
| Undo / Discard changes | Undo restores the selected button. Discard changes removes all unsaved mappings in the current profile. |

Layout edits save automatically; device mappings need confirmation. Avoid saving device settings from another page while editing here.

## Back up, move and update

- Use **More → Export layout / Import layout** to back up or move the layout and its input bindings. Importing replaces the current layout.
- **More → Back up device** downloads a `.gp2040` backup. The app also backs up before saving mappings. To restore it, import the file under **Data Backup and Restoration** in Firmware settings. The import takes effect immediately.
- Local layouts and device backups live in the `data` folder beside the EXE. Copy it when moving to another computer. Before updating, exit through the tray, replace the EXE and keep `data` and your custom `locales` files.

## Choose a language

The language menu at the top offers English, 简体中文 and 日本語. **Auto** follows your browser preferences and falls back to English when no language matches. The tray menu follows your Windows user language.

To add a translation:

1. Copy `locales/en.json` to a new file, such as `locales/fr.json`, in the `locales` folder beside the EXE.
2. Change `locale` to `fr` and `name` to the menu label, such as `Français`.
3. Translate the text in `messages`. Keep the keys and placeholders such as `{0}` and `{1}`. Missing entries fall back to English.
4. Save as UTF-8 and refresh the page to select the new language. Restart the app to reload tray translations.

## Exit

Closing the browser leaves the app running in the tray. Double-click the tray icon to reopen the page, or right-click it and choose **Exit** to close the app. Check the hidden icons area if you cannot see it.

See the [release guide](docs/releasing.md) for build and release steps. Release notes belong in [CHANGELOG.md](CHANGELOG.md).
