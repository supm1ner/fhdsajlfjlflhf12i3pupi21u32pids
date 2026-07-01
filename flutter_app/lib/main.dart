import 'package:flutter/foundation.dart' show kIsWeb, defaultTargetPlatform, TargetPlatform;
import 'package:flutter/material.dart';
import 'package:window_manager/window_manager.dart';

import 'src/screens/home_screen.dart';
import 'src/screens/login_screen.dart';
import 'src/state/app_state.dart';
import 'src/theme/glass_theme.dart';

bool get _isDesktop =>
    !kIsWeb &&
    (defaultTargetPlatform == TargetPlatform.macOS ||
        defaultTargetPlatform == TargetPlatform.windows ||
        defaultTargetPlatform == TargetPlatform.linux);

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Desktop: give the window a sensible default + minimum size and a clean title bar.
  if (_isDesktop) {
    await windowManager.ensureInitialized();
    const options = WindowOptions(
      size: Size(1120, 740),
      minimumSize: Size(720, 520),
      center: true,
      title: 'Sunrise',
      titleBarStyle: TitleBarStyle.normal,
    );
    await windowManager.waitUntilReadyToShow(options, () async {
      await windowManager.show();
      await windowManager.focus();
    });
  }

  runApp(const SunriseApp());
}

class SunriseApp extends StatefulWidget {
  const SunriseApp({super.key});

  @override
  State<SunriseApp> createState() => _SunriseAppState();
}

class _SunriseAppState extends State<SunriseApp> {
  final AppState _state = AppState();

  @override
  void dispose() {
    _state.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Sunrise',
      debugShowCheckedModeBanner: false,
      theme: buildGlassTheme(),
      home: ListenableBuilder(
        listenable: Listenable.merge([_state, _state.call, _state.liveKit]),
        builder: (context, _) {
          return _state.phase == Phase.ready
              ? HomeScreen(state: _state)
              : LoginScreen(state: _state);
        },
      ),
    );
  }
}
