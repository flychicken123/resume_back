import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

public class Delivery {
    // Map to store driver information: driver_id -> hourly_rate
    private final Map<Integer, Double> drivers;
    // List to store delivery records
    private final List<DeliveryRecord> deliveries;

    public Delivery() {
        this.drivers = new HashMap<>();
        this.deliveries = new ArrayList<>();
    }

    /**
     * Add a new driver with their hourly rate.
     *
     * @param driverId Unique identifier for the driver
     * @param usdHourlyRate Driver's hourly rate in USD
     * @throws IllegalArgumentException if driver already exists or rate is invalid
     */
    public void addDriver(int driverId, double usdHourlyRate) {
        if (drivers.containsKey(driverId)) {
            throw new IllegalArgumentException("Driver with ID " + driverId + " already exists");
        }
        if (usdHourlyRate <= 0) {
            throw new IllegalArgumentException("Hourly rate must be greater than 0");
        }
        
        drivers.put(driverId, usdHourlyRate);
    }

    /**
     * Record a delivery with its start and end times.
     *
     * @param driverId ID of the driver who made the delivery
     * @param startTime When the delivery started
     * @param endTime When the delivery ended
     * @throws IllegalArgumentException if driver not found or times are invalid
     */
    public void recordDelivery(int driverId, LocalDateTime startTime, LocalDateTime endTime) {
        if (!drivers.containsKey(driverId)) {
            throw new IllegalArgumentException("Driver with ID " + driverId + " not found");
        }
        if (!endTime.isAfter(startTime)) {
            throw new IllegalArgumentException("End time must be after start time");
        }
        
        deliveries.add(new DeliveryRecord(driverId, startTime, endTime));
    }

    /**
     * Calculate total earnings for a specific driver.
     *
     * @param driverId ID of the driver
     * @return Total earnings in USD
     * @throws IllegalArgumentException if driver not found
     */
    public double getDriverEarnings(int driverId) {
        if (!drivers.containsKey(driverId)) {
            throw new IllegalArgumentException("Driver with ID " + driverId + " not found");
        }
        
        double hourlyRate = drivers.get(driverId);
        double totalHours = 0.0;
        
        for (DeliveryRecord delivery : deliveries) {
            if (delivery.getDriverId() == driverId) {
                double duration = java.time.Duration.between(
                    delivery.getStartTime(), 
                    delivery.getEndTime()
                ).toMinutes() / 60.0; // Convert to hours
                totalHours += duration;
            }
        }
        
        return totalHours * hourlyRate;
    }

    /**
     * Get all delivery records.
     *
     * @return List of delivery records
     */
    public List<DeliveryRecord> getAllDeliveries() {
        return new ArrayList<>(deliveries);
    }

    /**
     * Get all delivery records for a specific driver.
     *
     * @param driverId ID of the driver
     * @return List of delivery records for the driver
     * @throws IllegalArgumentException if driver not found
     */
    public List<DeliveryRecord> getDriverDeliveries(int driverId) {
        if (!drivers.containsKey(driverId)) {
            throw new IllegalArgumentException("Driver with ID " + driverId + " not found");
        }
        
        List<DeliveryRecord> driverDeliveries = new ArrayList<>();
        for (DeliveryRecord delivery : deliveries) {
            if (delivery.getDriverId() == driverId) {
                driverDeliveries.add(delivery);
            }
        }
        return driverDeliveries;
    }

    /**
     * Inner class to represent a delivery record
     */
    private static class DeliveryRecord {
        private final int driverId;
        private final LocalDateTime startTime;
        private final LocalDateTime endTime;

        public DeliveryRecord(int driverId, LocalDateTime startTime, LocalDateTime endTime) {
            this.driverId = driverId;
            this.startTime = startTime;
            this.endTime = endTime;
        }

        public int getDriverId() {
            return driverId;
        }

        public LocalDateTime getStartTime() {
            return startTime;
        }

        public LocalDateTime getEndTime() {
            return endTime;
        }
    }
} 