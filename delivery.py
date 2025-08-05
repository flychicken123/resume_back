from datetime import datetime
from typing import Dict, List, Optional

class Delivery:
    def __init__(self):
        # Dictionary to store driver information: {driver_id: hourly_rate}
        self.drivers: Dict[int, float] = {}
        # List to store delivery records: [(driver_id, start_time, end_time)]
        self.deliveries: List[tuple] = []

    def add_driver(self, driver_id: int, usd_hourly_rate: float) -> None:
        """
        Add a new driver with their hourly rate.
        
        Args:
            driver_id (int): Unique identifier for the driver
            usd_hourly_rate (float): Driver's hourly rate in USD
        """
        if driver_id in self.drivers:
            raise ValueError(f"Driver with ID {driver_id} already exists")
        if usd_hourly_rate <= 0:
            raise ValueError("Hourly rate must be greater than 0")
        
        self.drivers[driver_id] = usd_hourly_rate

    def record_delivery(self, driver_id: int, start_time: datetime, end_time: datetime) -> None:
        """
        Record a delivery with its start and end times.
        
        Args:
            driver_id (int): ID of the driver who made the delivery
            start_time (datetime): When the delivery started
            end_time (datetime): When the delivery ended
        """
        if driver_id not in self.drivers:
            raise ValueError(f"Driver with ID {driver_id} not found")
        if end_time <= start_time:
            raise ValueError("End time must be after start time")
        
        self.deliveries.append((driver_id, start_time, end_time))

    def get_driver_earnings(self, driver_id: int) -> float:
        """
        Calculate total earnings for a specific driver.
        
        Args:
            driver_id (int): ID of the driver
            
        Returns:
            float: Total earnings in USD
        """
        if driver_id not in self.drivers:
            raise ValueError(f"Driver with ID {driver_id} not found")
        
        hourly_rate = self.drivers[driver_id]
        total_hours = 0.0
        
        for d_id, start, end in self.deliveries:
            if d_id == driver_id:
                duration = (end - start).total_seconds() / 3600  # Convert to hours
                total_hours += duration
        
        return total_hours * hourly_rate

    def get_all_deliveries(self) -> List[tuple]:
        """
        Get all delivery records.
        
        Returns:
            List[tuple]: List of (driver_id, start_time, end_time) tuples
        """
        return self.deliveries.copy()

    def get_driver_deliveries(self, driver_id: int) -> List[tuple]:
        """
        Get all delivery records for a specific driver.
        
        Args:
            driver_id (int): ID of the driver
            
        Returns:
            List[tuple]: List of (driver_id, start_time, end_time) tuples
        """
        if driver_id not in self.drivers:
            raise ValueError(f"Driver with ID {driver_id} not found")
        
        return [(d_id, start, end) for d_id, start, end in self.deliveries if d_id == driver_id] 