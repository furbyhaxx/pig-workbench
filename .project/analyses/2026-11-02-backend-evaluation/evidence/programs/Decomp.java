import ghidra.app.script.GhidraScript;
import ghidra.app.decompiler.*;
import ghidra.program.model.listing.*;

public class Decomp extends GhidraScript {
    @Override
    public void run() throws Exception {
        String name = getScriptArgs().length > 0 ? getScriptArgs()[0] : "main";
        DecompInterface d = new DecompInterface();
        d.openProgram(currentProgram);
        for (Function f : currentProgram.getFunctionManager().getFunctions(true)) {
            if (!f.getName().equals(name)) continue;
            DecompileResults r = d.decompileFunction(f, 60, monitor);
            println("WB-PSEUDO-BEGIN " + f.getName() + " @ " + f.getEntryPoint());
            println(r.decompileCompleted() ? r.getDecompiledFunction().getC() : ("FAILED: " + r.getErrorMessage()));
            println("WB-PSEUDO-END");
        }
        d.dispose();
    }
}
