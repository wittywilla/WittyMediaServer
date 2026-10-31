// Runs the window with no one at the keyboard, for automated checks.
#pragma once

#include <QString>

namespace wmp {

class MainWindow;

// Steps, separated by semicolons:
//   open <path>         key <name>          wait <seconds>      seek <seconds>
//   grab <file.png>     windowgrab <file>   report <file.json>  volume <0..1>
//   engine <vlc|built-in>   settingsgrab <file>   quit
void runScript(MainWindow *window, const QString &steps);

} // namespace wmp
