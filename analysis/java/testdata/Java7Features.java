import java.util.List;
import java.util.ArrayList;
import java.io.IOException;

public class Java7Features {
    private List<String> items = new ArrayList<>();

    public void processItems(String type) {
        switch (type) {
            case "TYPE_A":
                processTypeA();
                break;
            case "TYPE_B":
                processTypeB();
                break;
            default:
                processDefault();
        }

        int value = 0b1010_1100;
        long number = 1_000_000L;
    }

    public void readFile(String filename) throws IOException {
        try (java.io.BufferedReader reader = new java.io.BufferedReader(
                new java.io.FileReader(filename))) {
            String line = reader.readLine();
        }
    }

    private void processTypeA() {}
    private void processTypeB() {}
    private void processDefault() {}
}
