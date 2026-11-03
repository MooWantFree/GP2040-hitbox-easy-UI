using System;
using System.Runtime.InteropServices;
using System.Text;
public static class TrayProbe {
    private delegate bool EnumWindowProc(IntPtr window, IntPtr data);
    [DllImport("user32.dll")] private static extern bool EnumWindows(EnumWindowProc callback, IntPtr data);
    [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr window, out uint process);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern int GetClassName(IntPtr window, StringBuilder name, int count);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] private static extern IntPtr SendMessageTimeout(IntPtr window,uint message,UIntPtr wParam,IntPtr lParam,uint flags,uint timeout,out UIntPtr result);
    [StructLayout(LayoutKind.Sequential)] private struct Identifier { public uint Size; public IntPtr Window; public uint Id; public Guid Guid; }
    [StructLayout(LayoutKind.Sequential)] private struct Rect { public int Left,Top,Right,Bottom; }
    [DllImport("shell32.dll")] private static extern int Shell_NotifyIconGetRect(ref Identifier id, out Rect rect);
    public static IntPtr Find(uint process) {
        IntPtr found=IntPtr.Zero;
        EnumWindows((window,data)=>{
            uint owner; GetWindowThreadProcessId(window,out owner);
            if(owner!=process)return true;
            var name=new StringBuilder(128);GetClassName(window,name,name.Capacity);
            if(name.ToString()!="HitboxMap.Tray")return true;
            found=window;return false;
        },IntPtr.Zero);
        return found;
    }
    public static bool HasIcon(IntPtr window) {
        var id=new Identifier {Size=(uint)Marshal.SizeOf<Identifier>(),Window=window,Id=1};
        Rect rect;return Shell_NotifyIconGetRect(ref id,out rect)==0;
    }
    public static void Exit(uint process) {
        IntPtr window=Find(process);if(window==IntPtr.Zero)throw new InvalidOperationException("Test tray not found");
        UIntPtr result;
        if(SendMessageTimeout(window,0x111,new UIntPtr(2),IntPtr.Zero,2,3000,out result)==IntPtr.Zero)throw new InvalidOperationException("Tray exit timed out");
    }
}
